package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestResolveProviderConfigPrefersRequestOverEnv(t *testing.T) {
	t.Setenv("OPENAI_URL", "https://env.example/v1/chat/completions")
	t.Setenv("OPENAI_MODEL", "env-model")
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("LLM_PROVIDER", "")

	temp := 0.2
	body := chatRequest{
		Provider:    "openai-compatible",
		BaseURL:     "https://req.example/v1/chat/completions",
		Model:       "req-model",
		Temperature: &temp,
	}
	r := httptest.NewRequest("POST", "/api/gpt", nil)
	r.Header.Set("X-LLM-Api-Key", "req-key")

	cfg, ok := resolveProviderConfig(r, body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if cfg.BaseURL != "https://req.example/v1/chat/completions" {
		t.Errorf("BaseURL = %q, want request value", cfg.BaseURL)
	}
	if cfg.Model != "req-model" {
		t.Errorf("Model = %q, want req-model", cfg.Model)
	}
	if cfg.APIKey != "req-key" {
		t.Errorf("APIKey = %q, want header value", cfg.APIKey)
	}
	if cfg.Temperature == nil || *cfg.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", cfg.Temperature)
	}
}

func TestResolveProviderConfigFallsBackToEnv(t *testing.T) {
	t.Setenv("OPENAI_URL", "https://env.example/v1/chat/completions")
	t.Setenv("OPENAI_MODEL", "env-model")
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("LLM_PROVIDER", "")

	r := httptest.NewRequest("POST", "/api/gpt", nil)

	cfg, ok := resolveProviderConfig(r, chatRequest{})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if cfg.Provider != ProviderOpenAICompatible {
		t.Errorf("Provider = %q, want default openai-compatible", cfg.Provider)
	}
	if cfg.BaseURL != "https://env.example/v1/chat/completions" {
		t.Errorf("BaseURL = %q, want env value", cfg.BaseURL)
	}
	if cfg.Model != "env-model" || cfg.APIKey != "env-key" {
		t.Errorf("Model/APIKey = %q/%q, want env values", cfg.Model, cfg.APIKey)
	}
	if cfg.Temperature != nil {
		t.Errorf("Temperature = %v, want nil (omitted) when unset", cfg.Temperature)
	}
}

func TestResolveProviderConfigMissingIsNotOK(t *testing.T) {
	t.Setenv("OPENAI_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_API_KEY", "")

	r := httptest.NewRequest("POST", "/api/gpt", nil)
	if _, ok := resolveProviderConfig(r, chatRequest{}); ok {
		t.Fatal("expected ok=false when no config is available")
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

func TestDeriveModelsURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1/chat/completions":  "https://api.openai.com/v1/models",
		"http://localhost:11434/v1/chat/completions":  "http://localhost:11434/v1/models",
		"https://api.openai.com/v1":                   "https://api.openai.com/v1/models",
		"https://api.openai.com/v1/chat/completions/": "https://api.openai.com/v1/models",
	}
	for in, want := range cases {
		if got := deriveModelsURL(in); got != want {
			t.Errorf("deriveModelsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListModelsParsesAndSorts(t *testing.T) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := ProviderConfig{BaseURL: srv.URL + "/v1/chat/completions", APIKey: "bad"}
	if _, err := (openAICompatibleProvider{}).ListModels(context.Background(), cfg); err == nil {
		t.Fatal("expected error on non-200 models response")
	}
}
