package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
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

// ProviderConfig is the endpoint configuration resolved for a single chat
// request. Each field is taken from the request when present and otherwise
// falls back to the server-side .env, so existing .env-only setups keep working.
type ProviderConfig struct {
	Provider ProviderKind
	BaseURL  string
	Model    string
	APIKey   string // raw key; auth-header formatting is the provider's job
	// Temperature is nil unless the request set one explicitly. We omit it from
	// the payload when nil so the provider applies its own default — some models
	// (e.g. OpenAI reasoning models) reject any non-default temperature.
	Temperature *float64
}

// chatRequest is the JSON body of POST /api/gpt. The provider override fields
// are optional; the API key is read from the X-Llm-Api-Key header (not the
// body) so it stays out of request logs.
type chatRequest struct {
	Messages                  []map[string]interface{} `json:"messages"`
	TraceInfo                 map[string]interface{}   `json:"traceInfo"`
	SelectedGitHubRoutineKeys []string                 `json:"selectedGitHubRoutineKeys"`

	Provider    string   `json:"provider"`
	BaseURL     string   `json:"baseURL"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature"`
}

const providerConfigHelp = "[Daisen Bot is not configured] No model, endpoint, or " +
	"API key was provided.\n" +
	"Open Settings (the gear in the chat panel) to enter your provider, model, " +
	"base URL, and API key, or create a \".env\" next to the server with:\n" +
	"```\n" +
	"OPENAI_URL=\"https://api.openai.com/v1/chat/completions\"\n" +
	"OPENAI_MODEL=\"gpt-4o\"\n" +
	"OPENAI_API_KEY=\"sk-proj-XXXXXXXXXXXX\"\n" +
	"```\n"

// httpChatProxy proxies a chat request to the configured LLM provider. It
// resolves the provider config (request overrides .env), assembles the
// trace/repo/system context, then lets the selected provider build the
// outbound request and relay the response.
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

	provider, err := newChatProvider(cfg.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	messages := assembleMessages(r.Context(), body, s.traceReader)

	outReq, err := provider.BuildRequest(r.Context(), cfg, messages)
	if err != nil {
		http.Error(w, "Failed to build provider request: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	resp, err := guardedLLMClient.Do(outReq)
	if err != nil {
		http.Error(w, "Failed to contact LLM provider: "+err.Error(),
			http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if err := provider.RelayResponse(w, resp); err != nil {
		log.Println("Failed to relay LLM response:", err)
	}
}

// resolveEndpointKey decides the base URL and API key for an outbound request
// and enforces one invariant for shared deployments: the server's default key
// (from .env) is only ever sent to the server's own endpoint, never to a
// client-supplied URL — otherwise a user could point baseURL at a server they
// control and capture the server's secret. A client endpoint with no client key
// is sent keyless (works for local no-auth servers; an endpoint that needs a key
// answers 401).
func resolveEndpointKey(headerKey, reqBaseURL string) (baseURL, apiKey string, ok bool) {
	serverURL := os.Getenv("OPENAI_URL")
	serverKey := os.Getenv("OPENAI_API_KEY")

	switch {
	case headerKey != "":
		// A client key pairs with the client endpoint (or the server's if unset).
		baseURL, apiKey = firstNonEmpty(reqBaseURL, serverURL), headerKey
	case reqBaseURL == "" || reqBaseURL == serverURL:
		// The server's own endpoint: the server key may be used here.
		baseURL, apiKey = serverURL, serverKey
	default:
		// A different, client-supplied endpoint with no key: never attach the
		// server key to it.
		baseURL, apiKey = reqBaseURL, ""
	}

	if baseURL == "" {
		return "", "", false
	}
	return baseURL, apiKey, true
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
// than only up front) closes two SSRF gaps that an up-front URL check misses:
// HTTP redirects to an internal Location (each redirect dials through here) and
// DNS rebinding (the kernel never re-resolves the hostname, so it can't be
// pointed at an internal address between check and connect). Opt out for local
// providers with DAISEN_ALLOW_PRIVATE_LLM_URL=1.
func guardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	if allowPrivateLLMHosts() {
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
	// Dial the already-vetted IP, not the hostname, so there is no second
	// resolution that could rebind to an internal address.
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

// guardedLLMClient is the single HTTP client used for all outbound LLM calls
// (chat and model listing). TLS still verifies against the request hostname
// (SNI), while connections are pinned to vetted public IPs by the dialer.
var guardedLLMClient = &http.Client{
	Transport: &http.Transport{
		DialContext:         guardedDialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	},
}

// resolveProviderConfig builds a ProviderConfig from the request, falling back
// to the server-side .env. The API key arrives in the X-Llm-Api-Key header; the
// endpoint/key pairing rules live in resolveEndpointKey. It returns ok=false
// when no usable endpoint or model can be determined.
func resolveProviderConfig(r *http.Request, body chatRequest) (ProviderConfig, bool) {
	_ = godotenv.Load(".env")

	baseURL, apiKey, ok := resolveEndpointKey(r.Header.Get("X-Llm-Api-Key"), body.BaseURL)
	if !ok {
		return ProviderConfig{}, false
	}

	cfg := ProviderConfig{
		Provider: ProviderKind(firstNonEmpty(
			body.Provider, os.Getenv("LLM_PROVIDER"), string(ProviderOpenAICompatible))),
		BaseURL:     baseURL,
		Model:       firstNonEmpty(body.Model, os.Getenv("OPENAI_MODEL")),
		APIKey:      apiKey,
		Temperature: body.Temperature,
	}

	if cfg.Model == "" {
		return cfg, false
	}
	return cfg, true
}

// assembleMessages prepends the trace-context CSV, GitHub routine files, and the
// system prompt to the conversation. The result is provider-independent; each
// provider serializes it into its own wire format. Missing optional context
// files (componentgithubroutine.json, beforehandprompt.txt) degrade gracefully
// rather than failing the request.
func assembleMessages(
	ctx context.Context,
	body chatRequest,
	traceReader *SQLiteTraceReader,
) []map[string]interface{} {
	combinedTraceHeader := buildAkitaTraceHeader(traceReader, body.TraceInfo)

	combinedRepoHeader := ""
	urlList, err := getRoutineURLList("componentgithubroutine.json", body.SelectedGitHubRoutineKeys)
	if err != nil {
		log.Println("Skipping GitHub routine context:", err)
	} else {
		combinedRepoHeader = buildCombinedRepoHeader(ctx, urlList)
	}

	messages := body.Messages
	if len(messages) > 0 {
		if contentArr, ok := messages[len(messages)-1]["content"].([]interface{}); ok && len(contentArr) > 0 {
			if firstContent, ok := contentArr[0].(map[string]interface{}); ok {
				firstText, _ := firstContent["text"].(string)
				firstContent["text"] = combinedTraceHeader + combinedRepoHeader + firstText
			}
		}
	}

	if len(messages) == 0 || messages[0]["role"] != "system" {
		if loadedTextBytes, err := os.ReadFile("beforehandprompt.txt"); err != nil {
			log.Println("Skipping system prompt:", err)
		} else {
			systemMsg := map[string]interface{}{
				"role": "system",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": string(loadedTextBytes),
					},
				},
			}
			messages = append([]map[string]interface{}{systemMsg}, messages...)
		}
	}

	return messages
}

// ChatProvider serializes assembled messages into a provider-specific HTTP
// request and relays the provider's response to the client. OpenAI-compatible
// is the only implementation today; adding Anthropic is a new ChatProvider, not
// a change to the handler or context assembly.
type ChatProvider interface {
	BuildRequest(
		ctx context.Context, cfg ProviderConfig, messages []map[string]interface{},
	) (*http.Request, error)
	RelayResponse(w http.ResponseWriter, resp *http.Response) error
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

func (openAICompatibleProvider) BuildRequest(
	ctx context.Context, cfg ProviderConfig, messages []map[string]interface{},
) (*http.Request, error) {
	payload := map[string]interface{}{
		"model":    cfg.Model,
		"messages": messages,
	}
	// Only send temperature when explicitly set; otherwise let the model use its
	// default (reasoning models reject anything but the default).
	if cfg.Temperature != nil {
		payload["temperature"] = *cfg.Temperature
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.BaseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", bearerToken(cfg.APIKey))
	}
	return req, nil
}

func (openAICompatibleProvider) RelayResponse(w http.ResponseWriter, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(body)
	return err
}

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

	body, _ := io.ReadAll(resp.Body)
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
// listing URL: ".../v1/chat/completions" -> ".../v1/models". A URL that does not
// end in /chat/completions just gets "/models" appended.
func deriveModelsURL(chatURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(chatURL), "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return strings.TrimSuffix(trimmed, "/chat/completions") + "/models"
	}
	return trimmed + "/models"
}

// httpListModels proxies the provider's model-discovery endpoint so the frontend
// can populate a model picker without cross-origin issues. The base URL and
// provider come from the request (falling back to .env), the key from the
// X-Llm-Api-Key header, with the same key/endpoint safety rules as chat.
func (s *Server) httpListModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"baseURL"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	_ = godotenv.Load(".env")
	baseURL, apiKey, ok := resolveEndpointKey(r.Header.Get("X-Llm-Api-Key"), body.BaseURL)
	if !ok {
		http.Error(w, "No base URL configured", http.StatusBadRequest)
		return
	}
	if err := guardLLMURL(baseURL); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	cfg := ProviderConfig{
		Provider: ProviderKind(firstNonEmpty(
			body.Provider, os.Getenv("LLM_PROVIDER"), string(ProviderOpenAICompatible))),
		BaseURL: baseURL,
		APIKey:  apiKey,
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

// httpLLMCapabilities reports whether the server has a usable .env default and,
// if so, the default model/base URL, so the frontend knows whether the user
// must supply credentials. It never returns the API key itself.
func (s *Server) httpLLMCapabilities(w http.ResponseWriter, _ *http.Request) {
	_ = godotenv.Load(".env")

	baseURL := os.Getenv("OPENAI_URL")
	model := os.Getenv("OPENAI_MODEL")
	hasServerDefault := os.Getenv("OPENAI_API_KEY") != "" && baseURL != "" && model != ""

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"hasServerDefault": hasServerDefault,
		"defaultModel":     model,
		"defaultBaseURL":   baseURL,
		"providers":        []string{string(ProviderOpenAICompatible)},
	}); err != nil {
		http.Error(w, "Failed to encode JSON: "+err.Error(), http.StatusInternalServerError)
	}
}

// bearerToken formats a raw API key as an Authorization header value. It accepts
// keys that already carry the "Bearer " prefix (as the legacy .env did) and adds
// it otherwise, so raw keys from the frontend and prefixed .env keys both work.
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
