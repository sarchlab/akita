package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sarchlab/akita/v5/sourcefs"
)

func testSource(t *testing.T, files map[string]string) *sourcefs.Source {
	t.Helper()
	mapFS := fstest.MapFS{}
	for p, content := range files {
		mapFS[p] = &fstest.MapFile{Data: []byte(content)}
	}
	src, err := sourcefs.NewSource(mapFS, []string{"example.com/m"})
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	return src
}

func TestCodeSearch(t *testing.T) {
	src := testSource(t, map[string]string{
		"example.com/m/mem/read.go":  "package mem\ntype ReadReq struct{}\n\nfunc handle() {}\n",
		"example.com/m/mem/write.go": "package mem\ntype WriteReq struct{}\n",
		"example.com/m/noc/conn.go":  "package noc\n// ReadReq is referenced here\n",
	})

	out, err := runCodeSearch(src, `ReadReq`, "")
	if err != nil {
		t.Fatalf("runCodeSearch: %v", err)
	}
	// Two files mention ReadReq; results are file:line.
	if !strings.Contains(out, "example.com/m/mem/read.go:2:") {
		t.Errorf("missing definition match:\n%s", out)
	}
	if !strings.Contains(out, "example.com/m/noc/conn.go:2:") {
		t.Errorf("missing reference match:\n%s", out)
	}

	// path_contains narrows the search.
	out, _ = runCodeSearch(src, `ReadReq`, "mem/")
	if strings.Contains(out, "noc/conn.go") {
		t.Errorf("path_contains=mem/ should exclude noc:\n%s", out)
	}

	// No match.
	if out, _ := runCodeSearch(src, `DoesNotExist`, ""); !strings.Contains(out, "No matches") {
		t.Errorf("expected no-match message, got:\n%s", out)
	}

	// Invalid regex is an error the model can recover from.
	if _, err := runCodeSearch(src, `(unclosed`, ""); err == nil {
		t.Errorf("expected error for invalid regex")
	}
}

func TestCodeSearchCapsMatches(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("package big\n")
	for i := 0; i < codeSearchMaxMatches+50; i++ {
		sb.WriteString("// needle here\n")
	}
	src := testSource(t, map[string]string{"example.com/m/big.go": sb.String()})

	out, err := runCodeSearch(src, "needle", "")
	if err != nil {
		t.Fatalf("runCodeSearch: %v", err)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation note for a flood of matches:\n%s", out[:min(len(out), 400)])
	}
	if got := strings.Count(out, "needle here"); got > codeSearchMaxMatches {
		t.Errorf("returned %d matches, want <= %d", got, codeSearchMaxMatches)
	}
}

func TestCodeSearchEmptySource(t *testing.T) {
	out, err := runCodeSearch(&sourcefs.Source{}, "anything", "")
	if err != nil {
		t.Fatalf("runCodeSearch: %v", err)
	}
	if !strings.Contains(out, "No simulator source is recorded") {
		t.Errorf("expected no-source message, got:\n%s", out)
	}
}

func TestCodeRead(t *testing.T) {
	src := testSource(t, map[string]string{
		"example.com/m/mem/read.go": "package mem\n\ntype ReadReq struct{}\n\nfunc handle() {}\n",
	})

	// Full small file is numbered.
	out, err := runCodeRead(src, "example.com/m/mem/read.go", 0, 0)
	if err != nil {
		t.Fatalf("runCodeRead: %v", err)
	}
	if !strings.Contains(out, "1\tpackage mem") || !strings.Contains(out, "3\ttype ReadReq struct{}") {
		t.Errorf("expected numbered lines:\n%s", out)
	}
	if !strings.Contains(out, "of 5):") {
		t.Errorf("expected total-line count of 5:\n%s", out)
	}

	// Range selects a window.
	out, _ = runCodeRead(src, "example.com/m/mem/read.go", 3, 3)
	if !strings.Contains(out, "3\ttype ReadReq struct{}") || strings.Contains(out, "package mem") {
		t.Errorf("range 3-3 should show only line 3:\n%s", out)
	}

	// Missing file is reported (not an error), with a hint.
	out, _ = runCodeRead(src, "example.com/m/nope.go", 0, 0)
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not-found message:\n%s", out)
	}

	// Invalid path is rejected.
	if _, err := runCodeRead(src, "../escape", 0, 0); err == nil {
		t.Errorf("expected error for traversal path")
	}
}

func TestCodeReadDefaultWindowTruncates(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < codeReadDefaultLines+100; i++ {
		sb.WriteString("line\n")
	}
	src := testSource(t, map[string]string{"example.com/m/long.go": sb.String()})

	out, err := runCodeRead(src, "example.com/m/long.go", 0, 0)
	if err != nil {
		t.Fatalf("runCodeRead: %v", err)
	}
	if !strings.Contains(out, "more lines; pass start_line/end_line") {
		t.Errorf("expected a 'more lines' note for a long file:\n%s", out[:min(len(out), 300)])
	}
}

// TestHTTPChatProxyCodeToolsSSE drives the whole /api/gpt agent loop against a
// mock provider that scripts code_search -> code_read -> answer over a seeded
// codeSource, asserting the SSE step/observation/message sequence. No API key.
func TestHTTPChatProxyCodeToolsSSE(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1")

	src := testSource(t, map[string]string{
		"example.com/m/mem/read.go": "package mem\ntype ReadReq struct{}\n",
	})

	calls := 0
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[`+
				`{"id":"c1","type":"function","function":{"name":"code_search",`+
				`"arguments":"{\"reason\":\"find ReadReq\",\"query\":\"ReadReq\"}"}}]}}]}`)
		case 2:
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[`+
				`{"id":"c2","type":"function","function":{"name":"code_read",`+
				`"arguments":"{\"reason\":\"read it\",\"path\":\"example.com/m/mem/read.go\"}"}}]}}]}`)
		default:
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":`+
				`"ReadReq is a memory read request type."}}]}`)
		}
	}))
	defer llm.Close()

	s := &Server{codeSource: src}
	reqBody, err := json.Marshal(map[string]interface{}{
		"provider": "openai-compatible",
		"baseURL":  llm.URL,
		"model":    "mock",
		"messages": []map[string]interface{}{
			{"role": "user", "content": []map[string]interface{}{{"type": "text", "text": "what is ReadReq?"}}},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest("POST", "/api/gpt", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	s.httpChatProxy(rec, req)

	out := rec.Body.String()
	for _, want := range []string{
		`"type":"step"`, "code_search", "code_read",
		"example.com/m/mem/read.go", "memory read request", `"type":"done"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("SSE stream missing %q\nstream:\n%s", want, out)
		}
	}
}
