package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// ProviderKind identifies the wire protocol used to talk to an LLM endpoint.
// Only OpenAI-compatible endpoints are supported today; the seam below lets us
// add native shapes (e.g. Anthropic) later without touching the handler or the
// trace-context assembly.
type ProviderKind string

const (
	// ProviderOpenAICompatible speaks the OpenAI /chat/completions protocol.
	// This reaches OpenAI itself plus every OpenAI-compatible endpoint
	// (Azure, Groq, Mistral, DeepSeek, OpenRouter, local Ollama/LM Studio/vLLM,
	// ...), which is the broadest reach for the least code.
	ProviderOpenAICompatible ProviderKind = "openai-compatible"
)

// ProviderConfig is the endpoint configuration for a single chat request. It is
// built entirely from the request: Daisen holds no server-side credentials.
type ProviderConfig struct {
	Provider ProviderKind
	BaseURL  string
	Model    string
	APIKey   string // raw key; may be empty for keyless local servers
	// Temperature is nil unless the request set one explicitly. We omit it from
	// the payload when nil so the provider applies its own default — some models
	// (e.g. OpenAI reasoning models) reject any non-default temperature.
	Temperature *float64
}

// chatRequest is the JSON body of POST /api/gpt. The provider/base URL/model
// come from the body; the API key is read from the X-Llm-Api-Key header (not the
// body) so it stays out of request logs.
type chatRequest struct {
	Messages  []map[string]interface{} `json:"messages"`
	TraceInfo map[string]interface{}   `json:"traceInfo"`

	Provider    string   `json:"provider"`
	BaseURL     string   `json:"baseURL"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature"`
}

const providerConfigHelp = "[Daisen Bot is not configured] No model or endpoint was provided.\n" +
	"Open Settings (the gear in the chat panel) and enter your provider, model, " +
	"base URL, and API key.\n"

// httpChatProxy runs DaisenBot's streamed, multi-step tool-calling loop. It reads
// the provider config from the request, assembles the agent system prompt + trace
// context, and streams the loop's steps and answer back as Server-Sent Events.
func (s *Server) httpChatProxy(w http.ResponseWriter, r *http.Request) {
	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	cfg, ok := resolveProviderConfig(r, body)
	if !ok {
		http.Error(w, providerConfigHelp, http.StatusBadRequest)
		return
	}

	if err := guardLLMURL(cfg.BaseURL); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if _, err := newChatProvider(cfg.Provider); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.runAgentSSE(w, r, cfg, assembleAgentMessages(body, s.traceReader))
}

// resolveProviderConfig builds a ProviderConfig from the request. The endpoint,
// model, and provider come from the body; the API key comes from the
// X-Llm-Api-Key header. The key may be empty (keyless local servers); the
// endpoint and model are required.
func resolveProviderConfig(r *http.Request, body chatRequest) (ProviderConfig, bool) {
	cfg := ProviderConfig{
		Provider:    ProviderKind(firstNonEmpty(body.Provider, string(ProviderOpenAICompatible))),
		BaseURL:     body.BaseURL,
		Model:       body.Model,
		APIKey:      r.Header.Get("X-Llm-Api-Key"),
		Temperature: body.Temperature,
	}

	if cfg.BaseURL == "" || cfg.Model == "" {
		return cfg, false
	}
	return cfg, true
}

// allowPrivateLLMHosts reports whether the operator has opted into letting the
// LLM endpoint be a private/loopback host. This is needed for local providers
// (Ollama, LM Studio, vLLM) but unsafe on a shared instance, so it is off by
// default and enabled with DAISEN_ALLOW_PRIVATE_LLM_URL=1.
func allowPrivateLLMHosts() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DAISEN_ALLOW_PRIVATE_LLM_URL"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// dialTargetIsProxy reports whether addr (the host:port being dialed) is one of
// the configured HTTP(S) proxies. When the transport routes a request through a
// proxy it dials the proxy — which may legitimately be on a private network — so
// that dial is allowed without the private-IP check; anything else is a direct
// connection to the LLM endpoint and is still validated. This is decided per
// dial so that a proxy which applies to some requests but not this one (NO_PROXY
// match, scheme mismatch) does not disable validation for the direct request.
func dialTargetIsProxy(addr string) bool {
	aHost, _, err := net.SplitHostPort(addr)
	if err != nil {
		aHost = addr
	}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			continue
		}
		if !strings.Contains(v, "://") {
			v = "http://" + v
		}
		pu, err := url.Parse(v)
		if err != nil {
			continue
		}
		if pu.Host == addr || (pu.Hostname() != "" && pu.Hostname() == aHost) {
			return true
		}
	}
	return false
}

func isInternalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// guardLLMURL rejects endpoints that resolve to private, loopback, or
// link-local addresses, so a shared Daisen instance can't be turned into an
// SSRF tool against internal services or cloud metadata endpoints (e.g.
// 169.254.169.254). Set DAISEN_ALLOW_PRIVATE_LLM_URL=1 to permit local
// endpoints when running Daisen for yourself.
func guardLLMURL(rawURL string) error {
	if allowPrivateLLMHosts() {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported base URL scheme %q", u.Scheme)
	}

	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot resolve base URL host %q: %w", host, err)
	}
	for _, ip := range ips {
		if isInternalIP(ip) {
			return fmt.Errorf("base URL host %q resolves to a private/loopback address; "+
				"set DAISEN_ALLOW_PRIVATE_LLM_URL=1 to allow local endpoints", host)
		}
	}
	return nil
}

// guardedDialContext resolves the target, rejects it if any address is internal,
// and then dials a vetted IP literal directly. Validating at dial time (rather
// than only up front) closes two SSRF gaps an up-front URL check misses: HTTP
// redirects to an internal Location (each redirect dials through here) and DNS
// rebinding (the kernel never re-resolves the hostname, so it can't be pointed at
// an internal address between check and connect). The check is skipped only when
// this dial targets a configured proxy (a direct dial to the endpoint is always
// validated, even if a proxy is set for other requests) or for local providers
// via DAISEN_ALLOW_PRIVATE_LLM_URL=1.
func guardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	if allowPrivateLLMHosts() || dialTargetIsProxy(addr) {
		return dialer.DialContext(ctx, network, addr)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isInternalIP(ip) {
			return nil, fmt.Errorf("refusing to connect to internal address %s for host %q", ip, host)
		}
	}
	// All addresses are vetted; dial them by IP (not the hostname, so there is no
	// second resolution that could rebind), falling back across addresses like
	// the default dialer when one is unreachable.
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no addresses found for host %q", host)
	}
	return nil, lastErr
}

// llmProxyFromEnvironment is the proxy resolver (a variable so tests can inject
// a decision without depending on http.ProxyFromEnvironment's process-wide
// caching of the environment).
var llmProxyFromEnvironment = http.ProxyFromEnvironment

// proxyForLLMRequest applies the standard HTTP(S)_PROXY rules. For a request
// that will be proxied, the proxy performs its own DNS resolution and connection
// — the dialer can't pin the IP — so the target host is re-validated here before
// proxying. This narrows the rebinding window; final egress control for proxied
// requests is delegated to the proxy.
func proxyForLLMRequest(req *http.Request) (*url.URL, error) {
	proxyURL, err := llmProxyFromEnvironment(req)
	if err != nil || proxyURL == nil {
		return proxyURL, err // direct request — the dialer validates and pins
	}
	if err := guardLLMURL(req.URL.String()); err != nil {
		return nil, err
	}
	return proxyURL, nil
}

// Bounds on untrusted, client-selected provider responses (vars so tests can
// shrink them).
var (
	// maxChatResponseBytes caps the relayed chat response so a hostile endpoint
	// can't exhaust server memory with an unbounded body.
	maxChatResponseBytes int64 = 32 << 20 // 32 MiB
	// maxModelsResponseBytes caps the model-list response before unmarshalling.
	maxModelsResponseBytes int64 = 8 << 20 // 8 MiB
)

// guardedLLMClient is the single HTTP client used for all outbound LLM calls
// (chat and model listing). It honors HTTP(S)_PROXY, pins direct connections to
// vetted public IPs via the dialer, re-validates proxied and redirect targets,
// verifies TLS against the request hostname (SNI), and bounds how long an
// untrusted endpoint can stall the request.
var guardedLLMClient = &http.Client{
	Timeout: 10 * time.Minute, // overall ceiling for a single provider call
	Transport: &http.Transport{
		Proxy:       proxyForLLMRequest,
		DialContext: guardedDialContext,
		// Bound idle keep-alive sockets the way http.DefaultTransport does, so
		// cycling through many client-selected hosts can't accumulate idle
		// connections without limit.
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute, // bound "accepts but never replies"
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after %d redirects", len(via))
		}
		return guardLLMURL(req.URL.String())
	},
}

// ChatProvider discovers the models an endpoint advertises. OpenAI-compatible is
// the only implementation today; the agent loop itself speaks the OpenAI
// /chat/completions protocol directly (see callProvider in agentloop.go).
type ChatProvider interface {
	// ListModels returns the model IDs the endpoint advertises, or an error if
	// discovery is unsupported or fails.
	ListModels(ctx context.Context, cfg ProviderConfig) ([]string, error)
}

func newChatProvider(kind ProviderKind) (ChatProvider, error) {
	switch kind {
	case ProviderOpenAICompatible:
		return openAICompatibleProvider{}, nil
	default:
		return nil, &unsupportedProviderError{kind: kind}
	}
}

type unsupportedProviderError struct{ kind ProviderKind }

func (e *unsupportedProviderError) Error() string {
	return "Unsupported LLM provider: " + string(e.kind) +
		" (only \"openai-compatible\" is supported)"
}

// openAICompatibleProvider talks to any endpoint that implements the OpenAI
// /chat/completions API.
type openAICompatibleProvider struct{}

func (openAICompatibleProvider) ListModels(ctx context.Context, cfg ProviderConfig) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", deriveModelsURL(cfg.BaseURL), nil)
	if err != nil {
		return nil, err
	}
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", bearerToken(cfg.APIKey))
	}

	resp, err := guardedLLMClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxModelsResponseBytes))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models endpoint returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID != "" && isGeneralChatModel(m.ID) {
			models = append(models, m.ID)
		}
	}
	sort.Strings(models)
	return models, nil
}

// nonChatModelMarkers are substrings identifying model families that are not
// general-purpose chat models: audio (whisper/tts/transcribe/realtime), image
// (dall-e/image), embeddings, moderation/safety, rerankers, and legacy base
// completion models. The model field stays free-text, so a model filtered out
// here can still be entered by typing its exact id.
var nonChatModelMarkers = []string{
	"whisper", "tts", "transcribe", "audio", "realtime",
	"dall-e", "dall·e", "image",
	"embedding", "embed",
	"moderation", "guard", "rerank",
	"babbage", "davinci",
}

// isGeneralChatModel reports whether a model id looks like a general-purpose
// chat model, i.e. it carries none of the non-chat family markers.
func isGeneralChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, marker := range nonChatModelMarkers {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

// deriveModelsURL maps an OpenAI-compatible chat-completions URL to its model
// listing URL: ".../v1/chat/completions" -> ".../v1/models". It rewrites the
// path only, preserving the scheme, host, and any query string (e.g. Azure's
// ?api-version=...). A path that does not end in /chat/completions just gets
// "/models" appended.
func deriveModelsURL(chatURL string) string {
	u, err := url.Parse(strings.TrimSpace(chatURL))
	if err != nil {
		return chatURL // best effort; the request will surface the real error
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/chat/completions") {
		u.Path = strings.TrimSuffix(path, "/chat/completions") + "/models"
	} else {
		u.Path = path + "/models"
	}
	return u.String()
}

// httpListModels proxies the provider's model-discovery endpoint so the frontend
// can populate a model picker without cross-origin issues. The endpoint/provider
// come from the request body, the key from the X-Llm-Api-Key header.
func (s *Server) httpListModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"baseURL"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.BaseURL == "" {
		http.Error(w, "No base URL configured", http.StatusBadRequest)
		return
	}
	if err := guardLLMURL(body.BaseURL); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	cfg := ProviderConfig{
		Provider: ProviderKind(firstNonEmpty(body.Provider, string(ProviderOpenAICompatible))),
		BaseURL:  body.BaseURL,
		APIKey:   r.Header.Get("X-Llm-Api-Key"),
	}

	provider, err := newChatProvider(cfg.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	models, err := provider.ListModels(r.Context(), cfg)
	if err != nil {
		http.Error(w, "Failed to list models: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"models": models}); err != nil {
		http.Error(w, "Failed to encode JSON: "+err.Error(), http.StatusInternalServerError)
	}
}

// bearerToken formats a raw API key as an Authorization header value, adding the
// "Bearer " prefix only when the key does not already carry it.
func bearerToken(key string) string {
	k := strings.TrimSpace(key)
	if strings.HasPrefix(strings.ToLower(k), "bearer ") {
		return k
	}
	return "Bearer " + k
}

// firstNonEmpty returns the first argument that is not the empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
