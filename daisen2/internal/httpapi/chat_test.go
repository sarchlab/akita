package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBearerTokenNormalizesRawAndPrefixedKeys(t *testing.T) {
	cases := map[string]string{
		"sk-123":         "Bearer sk-123",
		"Bearer sk-123":  "Bearer sk-123",
		"bearer sk-123":  "bearer sk-123", // already prefixed (any case) -> left as-is
		"  sk-123  ":     "Bearer sk-123",
		"Bearer  sk-123": "Bearer  sk-123",
	}
	for in, want := range cases {
		if got := bearerToken(in); got != want {
			t.Errorf("bearerToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveProviderConfigUsesRequest(t *testing.T) {
	temp := 0.2
	body := chatRequest{
		Provider:    "openai-compatible",
		BaseURL:     "https://req.example/v1/chat/completions",
		Model:       "req-model",
		Temperature: &temp,
	}
	r := httptest.NewRequest("POST", "/api/gpt", nil)
	r.Header.Set("X-Llm-Api-Key", "req-key")

	cfg, ok := resolveProviderConfig(r, body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if cfg.BaseURL != "https://req.example/v1/chat/completions" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "req-model" {
		t.Errorf("Model = %q, want req-model", cfg.Model)
	}
	if cfg.APIKey != "req-key" {
		t.Errorf("APIKey = %q, want the header value", cfg.APIKey)
	}
	if cfg.Temperature == nil || *cfg.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", cfg.Temperature)
	}
}

func TestResolveProviderConfigRequiresEndpointAndModel(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/gpt", nil)
	if _, ok := resolveProviderConfig(r, chatRequest{}); ok {
		t.Fatal("expected ok=false when no endpoint/model is provided")
	}
	if _, ok := resolveProviderConfig(r, chatRequest{BaseURL: "https://x/v1/chat/completions"}); ok {
		t.Fatal("expected ok=false when the model is missing")
	}
}

func TestResolveProviderConfigAllowsKeylessEndpoint(t *testing.T) {
	body := chatRequest{BaseURL: "http://localhost:11434/v1/chat/completions", Model: "llama3"}
	r := httptest.NewRequest("POST", "/api/gpt", nil) // no key header

	cfg, ok := resolveProviderConfig(r, body)
	if !ok {
		t.Fatal("expected ok=true for a keyless endpoint")
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty (keyless)", cfg.APIKey)
	}
	if cfg.BaseURL != "http://localhost:11434/v1/chat/completions" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
}

func TestGuardLLMURLBlocksInternalHosts(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "")

	blocked := []string{
		"http://127.0.0.1:11434/v1/chat/completions",
		"http://localhost:11434/v1/chat/completions",
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://10.1.2.3/v1/chat/completions",
		"http://192.168.0.5/v1",
	}
	for _, u := range blocked {
		if err := guardLLMURL(u); err == nil {
			t.Errorf("guardLLMURL(%q) = nil, want blocked", u)
		}
	}
}

func TestGuardLLMURLAllowsPublicHost(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "")
	if err := guardLLMURL("https://8.8.8.8/v1/chat/completions"); err != nil {
		t.Errorf("guardLLMURL(public) = %v, want nil", err)
	}
}

func TestGuardLLMURLOptInAllowsPrivate(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1")
	if err := guardLLMURL("http://localhost:11434/v1/chat/completions"); err != nil {
		t.Errorf("guardLLMURL(opt-in) = %v, want nil", err)
	}
}

func TestGuardedLLMClientBlocksInternalServer(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "")
	// Clear proxy env so the dialer guard applies (a proxy would route the dial
	// to the proxy, not the test server).
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// The dialer (not just the up-front URL check) must refuse the connection,
	// which is what closes the redirect and DNS-rebinding gaps.
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := guardedLLMClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected the guarded client to refuse a connection to 127.0.0.1")
	}
}

func TestGuardedLLMClientAllowsWithOptIn(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := guardedLLMClient.Do(req)
	if err != nil {
		t.Fatalf("guarded client with opt-in: %v", err)
	}
	_ = resp.Body.Close()
}

func TestGuardedLLMClientHonorsProxyEnv(t *testing.T) {
	tr, ok := guardedLLMClient.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatal("guarded client must honor HTTP(S)_PROXY via Transport.Proxy")
	}
}

func TestGuardedLLMClientBoundsIdleConns(t *testing.T) {
	tr := guardedLLMClient.Transport.(*http.Transport)
	if tr.IdleConnTimeout == 0 || tr.MaxIdleConns == 0 {
		t.Fatal("guarded transport must bound idle connections (IdleConnTimeout/MaxIdleConns)")
	}
}

func TestGuardedLLMClientValidatesDirectDialWhenProxyNotUsed(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "")
	// A proxy is configured, but it is not this target (and it is for https while
	// the test server is http), so this request is dialed directly — the dialer
	// must still reject the internal address rather than skipping the check.
	t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := guardedLLMClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("a direct dial to 127.0.0.1 must be blocked even when a proxy is configured for other requests")
	}
}

func TestRelayResponseCapsBody(t *testing.T) {
	orig := maxChatResponseBytes
	defer func() { maxChatResponseBytes = orig }()
	maxChatResponseBytes = 5

	rec := httptest.NewRecorder()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("0123456789")),
	}
	if err := (openAICompatibleProvider{}).RelayResponse(rec, resp); err != nil {
		t.Fatalf("RelayResponse: %v", err)
	}
	if rec.Body.Len() != 5 {
		t.Errorf("relayed %d bytes, want the 5-byte cap", rec.Body.Len())
	}
}

func TestProxyForLLMRequestRevalidatesProxiedTarget(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "")
	orig := llmProxyFromEnvironment
	defer func() { llmProxyFromEnvironment = orig }()
	// Pretend every request is proxied (bypasses ProxyFromEnvironment caching).
	llmProxyFromEnvironment = func(*http.Request) (*url.URL, error) {
		return &url.URL{Scheme: "http", Host: "proxy.example:3128"}, nil
	}

	internal, _ := http.NewRequest("GET", "http://127.0.0.1:9/v1/models", nil)
	if _, err := proxyForLLMRequest(internal); err == nil {
		t.Error("a proxied request to an internal target must be rejected before proxying")
	}

	public, _ := http.NewRequest("GET", "https://8.8.8.8/v1/chat/completions", nil)
	if _, err := proxyForLLMRequest(public); err != nil {
		t.Errorf("a proxied request to a public target should be allowed: %v", err)
	}
}

func TestNewChatProviderRejectsUnknown(t *testing.T) {
	if _, err := newChatProvider(ProviderOpenAICompatible); err != nil {
		t.Fatalf("openai-compatible should be supported: %v", err)
	}
	if _, err := newChatProvider("anthropic"); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestOpenAICompatibleBuildRequest(t *testing.T) {
	temp := 0.5
	cfg := ProviderConfig{
		Provider:    ProviderOpenAICompatible,
		BaseURL:     "https://api.example/v1/chat/completions",
		Model:       "gpt-test",
		APIKey:      "sk-test",
		Temperature: &temp,
	}
	messages := []map[string]interface{}{
		{"role": "user", "content": []interface{}{
			map[string]interface{}{"type": "text", "text": "hi"},
		}},
	}

	req, err := openAICompatibleProvider{}.BuildRequest(context.Background(), cfg, messages)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if req.URL.String() != cfg.BaseURL {
		t.Errorf("URL = %q, want %q", req.URL.String(), cfg.BaseURL)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", got)
	}

	bodyBytes, _ := io.ReadAll(req.Body)
	var payload struct {
		Model       string                   `json:"model"`
		Temperature float64                  `json:"temperature"`
		Messages    []map[string]interface{} `json:"messages"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Model != "gpt-test" {
		t.Errorf("payload model = %q, want gpt-test", payload.Model)
	}
	if payload.Temperature != 0.5 {
		t.Errorf("payload temperature = %v, want 0.5", payload.Temperature)
	}
	if len(payload.Messages) != 1 {
		t.Errorf("payload messages = %d, want 1", len(payload.Messages))
	}
}

func TestOpenAICompatibleBuildRequestOmitsTemperatureWhenUnset(t *testing.T) {
	cfg := ProviderConfig{
		Provider: ProviderOpenAICompatible,
		BaseURL:  "https://api.example/v1/chat/completions",
		Model:    "o3", // reasoning models reject non-default temperature
		APIKey:   "sk-test",
	}

	req, err := openAICompatibleProvider{}.BuildRequest(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bodyBytes, _ := io.ReadAll(req.Body)
	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := payload["temperature"]; ok {
		t.Error("temperature must be omitted from the payload when unset")
	}
}

func TestOpenAICompatibleBuildRequestOmitsAuthWhenNoKey(t *testing.T) {
	cfg := ProviderConfig{
		Provider: ProviderOpenAICompatible,
		BaseURL:  "https://api.example/v1/chat/completions",
		Model:    "llama3",
	}

	req, err := openAICompatibleProvider{}.BuildRequest(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty when no key is set", got)
	}
}

func TestDeriveModelsURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1/chat/completions":  "https://api.openai.com/v1/models",
		"http://localhost:11434/v1/chat/completions":  "http://localhost:11434/v1/models",
		"https://api.openai.com/v1":                   "https://api.openai.com/v1/models",
		"https://api.openai.com/v1/chat/completions/": "https://api.openai.com/v1/models",
		// Query strings (e.g. Azure's api-version) are preserved on the path rewrite.
		"https://host/v1/chat/completions?api-version=2024-08-01": "https://host/v1/models?api-version=2024-08-01",
	}
	for in, want := range cases {
		if got := deriveModelsURL(in); got != want {
			t.Errorf("deriveModelsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListModelsParsesAndSorts(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1") // the test server runs on 127.0.0.1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q, want Bearer sk-test", got)
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"gpt-4o"},{"id":"gpt-3.5"},{"id":""},
			{"id":"whisper-1"},{"id":"text-embedding-3-large"}
		]}`))
	}))
	defer srv.Close()

	cfg := ProviderConfig{
		BaseURL: srv.URL + "/v1/chat/completions",
		APIKey:  "sk-test",
	}
	models, err := openAICompatibleProvider{}.ListModels(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	// Empty IDs and non-chat models (whisper, embeddings) are dropped, and the
	// result is sorted.
	want := []string{"gpt-3.5", "gpt-4o"}
	if len(models) != len(want) || models[0] != want[0] || models[1] != want[1] {
		t.Errorf("models = %v, want %v", models, want)
	}
}

func TestIsGeneralChatModel(t *testing.T) {
	chat := []string{
		"gpt-4o", "gpt-4.1-mini", "o3", "chatgpt-4o-latest",
		"gpt-3.5-turbo", "llama-3.3-70b-versatile",
	}
	for _, id := range chat {
		if !isGeneralChatModel(id) {
			t.Errorf("isGeneralChatModel(%q) = false, want true", id)
		}
	}

	notChat := []string{
		"whisper-1", "tts-1", "tts-1-hd", "dall-e-3",
		"text-embedding-3-large", "omni-moderation-latest",
		"gpt-4o-audio-preview", "gpt-4o-realtime-preview",
		"gpt-4o-transcribe", "davinci-002", "babbage-002",
		"gpt-image-1", "nomic-embed-text",
	}
	for _, id := range notChat {
		if isGeneralChatModel(id) {
			t.Errorf("isGeneralChatModel(%q) = true, want false", id)
		}
	}
}

func TestListModelsReportsHTTPError(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1") // the test server runs on 127.0.0.1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := ProviderConfig{BaseURL: srv.URL + "/v1/chat/completions", APIKey: "bad"}
	if _, err := (openAICompatibleProvider{}).ListModels(context.Background(), cfg); err == nil {
		t.Fatal("expected error on non-200 models response")
	}
}
