package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// seedAgentTrace inserts a tiny dataset into the tables created by
// newTestTraceReader (shared with componentinfo_test.go): two components, with
// L2Cache (id 1) owning two tasks and L1Cache (id 2) owning one.
func seedAgentTrace(t *testing.T, reader *SQLiteTraceReader) {
	t.Helper()
	stmts := []string{
		`INSERT INTO location (ID, Locale) VALUES (1, 'L2Cache'), (2, 'L1Cache')`,
		`INSERT INTO trace VALUES (1,0,'read','req',1,0,10)`,
		`INSERT INTO trace VALUES (2,0,'read','req',1,10,25)`,
		`INSERT INTO trace VALUES (3,0,'write','req',2,5,8)`,
	}
	for _, s := range stmts {
		if _, err := reader.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
}

func TestSanitizeReadonlySQL(t *testing.T) {
	rejected := []string{
		"",
		"DELETE FROM trace",
		"DROP TABLE trace",
		"INSERT INTO trace VALUES (1)",
		"UPDATE trace SET Kind='x'",
		"PRAGMA table_info(trace)",
		"ATTACH DATABASE 'x' AS y",
		"SELECT 1; DROP TABLE trace",
	}
	for _, q := range rejected {
		if _, err := sanitizeReadonlySQL(q, 100); err == nil {
			t.Errorf("expected %q to be rejected", q)
		}
	}

	if got, err := sanitizeReadonlySQL("SELECT * FROM trace", 100); err != nil {
		t.Errorf("plain SELECT rejected: %v", err)
	} else if !strings.Contains(strings.ToUpper(got), "LIMIT 100") {
		t.Errorf("expected injected LIMIT, got %q", got)
	}

	got, _ := sanitizeReadonlySQL("SELECT * FROM trace LIMIT 5", 100)
	if strings.Count(strings.ToUpper(got), "LIMIT") != 1 {
		t.Errorf("must not double-inject LIMIT: %q", got)
	}

	if _, err := sanitizeReadonlySQL("WITH x AS (SELECT 1) SELECT * FROM x", 100); err != nil {
		t.Errorf("WITH should be allowed: %v", err)
	}
}

func TestRunDataQuery(t *testing.T) {
	reader := newTestTraceReader(t)
	seedAgentTrace(t, reader)

	out, err := runDataQuery(context.Background(), reader,
		"SELECT loc.Locale, COUNT(*) AS n FROM trace t JOIN location loc ON t.Location = loc.ID "+
			"GROUP BY loc.Locale ORDER BY loc.Locale")
	if err != nil {
		t.Fatalf("data_query: %v", err)
	}
	if !strings.Contains(out, "L2Cache,2") {
		t.Errorf("expected L2Cache,2 in output, got:\n%s", out)
	}

	// A write must be refused before reaching the DB.
	if _, err := runDataQuery(context.Background(), reader, "DELETE FROM trace"); err == nil {
		t.Error("expected DELETE to be rejected")
	}
}

// TestRunDataQueryRejectsCTEWrite guards the P1: a write smuggled through a CTE
// passes the SELECT/WITH prefix check but must be rejected by the read-only
// connection (PRAGMA query_only) without mutating the trace.
func TestRunDataQueryRejectsCTEWrite(t *testing.T) {
	reader := newTestTraceReader(t)
	seedAgentTrace(t, reader)

	var before int
	if err := reader.QueryRow("SELECT COUNT(*) FROM trace").Scan(&before); err != nil {
		t.Fatalf("count before: %v", err)
	}

	// Starts with WITH, so it slips past the prefix filter, but it is a DELETE.
	if _, err := runDataQuery(context.Background(), reader,
		"WITH x AS (SELECT 1) DELETE FROM trace RETURNING ID"); err == nil {
		t.Error("expected CTE-wrapped DELETE to be rejected")
	}

	var after int
	if err := reader.QueryRow("SELECT COUNT(*) FROM trace").Scan(&after); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if after != before {
		t.Errorf("trace was mutated: %d rows before, %d after", before, after)
	}

	// A normal read still works after the read-only guard reset the connection.
	if _, err := runDataQuery(context.Background(), reader, "SELECT COUNT(*) FROM trace"); err != nil {
		t.Errorf("read query after guard failed: %v", err)
	}
}

// TestFormatRowsByteCap guards the P2: a single oversized cell must not blow past
// the byte cap into the model context.
func TestFormatRowsByteCap(t *testing.T) {
	reader := newTestTraceReader(t)
	seedAgentTrace(t, reader)

	const byteCap = 2048
	rows, err := reader.QueryContext(context.Background(),
		"SELECT hex(zeroblob(1000000))")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	out, err := formatRows(rows, dataQueryRowCap, byteCap)
	if err != nil {
		t.Fatalf("formatRows: %v", err)
	}
	if len(out) > byteCap+dataQueryMaxCellBytes+256 {
		t.Errorf("output exceeded cap: %d bytes", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected a truncation notice, got:\n%s", out[:min(len(out), 200)])
	}
}

// TestRunAgentLoop_MockProvider is the end-to-end "preview": a scripted mock LLM
// drives the real loop against a real trace DB. Turn 1 asks for a data_query;
// the loop runs the SQL; turn 2 returns the final answer from the observation.
func TestRunAgentLoop_MockProvider(t *testing.T) { //nolint:funlen // end-to-end test with scripted mock server
	// Allow the loopback httptest server through the SSRF-guarded dialer.
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1")
	reader := newTestTraceReader(t)
	seedAgentTrace(t, reader)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			if _, ok := req["tools"]; !ok {
				t.Error("expected tools to be offered on the first turn")
			}
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant",`+
				`"content":"Let me check the per-component task counts.",`+
				`"tool_calls":[{"id":"c1","type":"function","function":{"name":"data_query",`+
				`"arguments":"{\"sql\":\"SELECT loc.Locale, COUNT(*) AS n FROM trace t `+
				`JOIN location loc ON t.Location=loc.ID GROUP BY loc.Locale\"}"}}]}}]}`)
			return
		}
		// Second turn: the conversation now carries the tool result.
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant",`+
			`"content":"L2Cache handled 2 tasks; L1Cache handled 1."}}]}`)
	}))
	defer srv.Close()

	cfg := ProviderConfig{Provider: ProviderOpenAICompatible, BaseURL: srv.URL, Model: "mock"}
	messages := []map[string]interface{}{{"role": "user", "content": "How many tasks per component?"}}

	var events []agentEvent
	emit := func(ev agentEvent) { events = append(events, ev) }

	if err := runAgentLoop(context.Background(), cfg, messages, []agentTool{dataQueryTool(reader)}, emit); err != nil {
		t.Fatalf("runAgentLoop: %v", err)
	}

	var sawThinking, sawStep, sawObs, sawMsg bool
	for _, ev := range events {
		switch ev.Type {
		case "thinking":
			if strings.Contains(ev.Text, "per-component") {
				sawThinking = true
			}
		case "step":
			if ev.Tool == "data_query" {
				sawStep = true
			}
		case "observation":
			if strings.Contains(ev.Observation, "L2Cache") {
				sawObs = true
			}
		case "message":
			if strings.Contains(ev.Text, "L2Cache") {
				sawMsg = true
			}
		}
	}
	if !sawThinking {
		t.Error("expected a thinking event (model reasoning alongside the tool call)")
	}
	if !sawStep {
		t.Error("expected a data_query step event")
	}
	if !sawObs {
		t.Error("expected an observation carrying the queried data")
	}
	if !sawMsg {
		t.Error("expected a final message")
	}
	if calls != 2 {
		t.Errorf("expected 2 provider calls (tool turn + answer turn), got %d", calls)
	}
}

// TestHTTPChatProxyAgentSSE drives the whole endpoint (httpChatProxy with
// agent=true) against a mock provider and asserts the SSE stream carries the
// step → observation (real queried data) → message → done sequence.
func TestHTTPChatProxyAgentSSE(t *testing.T) {
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1")
	reader := newTestTraceReader(t)
	seedAgentTrace(t, reader)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[`+
				`{"id":"c1","type":"function","function":{"name":"data_query",`+
				`"arguments":"{\"sql\":\"SELECT loc.Locale, COUNT(*) AS n FROM trace t `+
				`JOIN location loc ON t.Location=loc.ID GROUP BY loc.Locale\"}"}}]}}]}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"L2Cache handled 2 tasks."}}]}`)
	}))
	defer srv.Close()

	s := &Server{traceReader: reader}
	reqBody, err := json.Marshal(map[string]interface{}{
		"agent":    true,
		"provider": "openai-compatible",
		"baseURL":  srv.URL,
		"model":    "mock",
		"messages": []map[string]interface{}{
			{"role": "user", "content": []map[string]interface{}{{"type": "text", "text": "tasks per component?"}}},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest("POST", "/api/gpt", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	s.httpChatProxy(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected SSE content-type, got %q\nbody: %s", ct, rec.Body.String())
	}
	out := rec.Body.String()
	for _, want := range []string{`"type":"step"`, "data_query", "L2Cache", `"type":"message"`, `"type":"done"`} {
		if !strings.Contains(out, want) {
			t.Errorf("SSE stream missing %q\nstream:\n%s", want, out)
		}
	}
}

// TestAgentCaptureRoundTrip exercises the Phase 5 viz round-trip end-to-end: the
// model asks for daisen_view, the loop emits a `render` event, the test (playing
// the browser) POSTs an image to /api/agent/capture, and the loop resumes with
// the image as a multimodal observation and produces the final answer.
func TestAgentCaptureRoundTrip(t *testing.T) { //nolint:funlen // end-to-end test with scripted mock server
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1")
	reader := newTestTraceReader(t)
	seedAgentTrace(t, reader)

	calls := 0
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		calls++
		w.Header().Set("Content-Type", "application/json")
		if !bytes.Contains(raw, []byte("image_url")) {
			// Turn 1: ask to render a view.
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[`+
				`{"id":"c1","type":"function","function":{"name":"daisen_view",`+
				`"arguments":"{\"reason\":\"see L2 timeline\",\"url\":\"/component?name=L2Cache\"}"}}]}}]}`)
			return
		}
		// Turn 2: the image is now in the conversation — answer.
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"The L2 timeline looks steady."}}]}`)
	}))
	defer llm.Close()

	s := &Server{traceReader: reader}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/gpt", s.httpChatProxy)
	mux.HandleFunc("/api/agent/capture", s.httpAgentCapture)
	daisen := httptest.NewServer(mux)
	defer daisen.Close()

	reqBody, err := json.Marshal(map[string]interface{}{
		"provider": "openai-compatible",
		"baseURL":  llm.URL,
		"model":    "mock",
		"messages": []map[string]interface{}{
			{"role": "user", "content": []map[string]interface{}{{"type": "text", "text": "look at L2Cache"}}},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(daisen.URL+"/api/gpt", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /api/gpt: %v", err)
	}
	defer resp.Body.Close()

	var sawRender, sawMessage bool
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		switch ev["type"] {
		case "render":
			sawRender = true
			id, _ := ev["captureId"].(string)
			if ev["renderKind"] != "view" || ev["url"] != "/component?name=L2Cache" {
				t.Errorf("unexpected render event: %v", ev)
			}
			// Play the browser: POST a (fake) captured image.
			capBody, err := json.Marshal(map[string]string{"id": id, "image": "data:image/png;base64,AAAA"})
			if err != nil {
				t.Errorf("marshal capture: %v", err)
				continue
			}
			capResp, err := http.Post(daisen.URL+"/api/agent/capture", "application/json", bytes.NewReader(capBody))
			if err != nil {
				t.Errorf("POST capture: %v", err)
				continue
			}
			capResp.Body.Close()
		case "message":
			sawMessage = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading SSE: %v", err)
	}

	if !sawRender {
		t.Error("expected a render event")
	}
	if !sawMessage {
		t.Error("expected a final message after the capture")
	}
	if calls != 2 {
		t.Errorf("expected 2 provider calls (render turn + answer turn), got %d", calls)
	}
}

// TestAgentLoopMultiToolImageOrdering guards the fix for the OpenAI 400: when one
// assistant turn makes several tool calls and one returns an image, every tool
// response must immediately follow the assistant message — the image user message
// must come AFTER all tool responses, not between them.
func TestAgentLoopMultiToolImageOrdering(t *testing.T) { //nolint:funlen // end-to-end test with scripted mock server
	t.Setenv("DAISEN_ALLOW_PRIVATE_LLM_URL", "1")
	reader := newTestTraceReader(t)
	seedAgentTrace(t, reader)

	var turn2 []interface{}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[`+
				`{"id":"c1","type":"function","function":{"name":"snap","arguments":"{}"}},`+
				`{"id":"c2","type":"function","function":{"name":"data_query",`+
				`"arguments":"{\"reason\":\"r\",\"sql\":\"SELECT 1\"}"}}`+
				`]}}]}`)
			return
		}
		turn2, _ = req["messages"].([]interface{})
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
	}))
	defer srv.Close()

	snap := agentTool{
		name:        "snap",
		description: "snap",
		parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		run: func(_ context.Context, _ map[string]interface{}) (toolResult, error) {
			return toolResult{text: "snapped", images: []string{"data:image/png;base64,AAAA"}}, nil
		},
	}

	cfg := ProviderConfig{Provider: ProviderOpenAICompatible, BaseURL: srv.URL, Model: "mock"}
	msgs := []map[string]interface{}{{"role": "user", "content": "snap and query"}}
	tools := []agentTool{snap, dataQueryTool(reader)}
	if err := runAgentLoop(context.Background(), cfg, msgs, tools, func(agentEvent) {}); err != nil {
		t.Fatalf("runAgentLoop: %v", err)
	}

	role := func(i int) string {
		m, _ := turn2[i].(map[string]interface{})
		s, _ := m["role"].(string)
		return s
	}
	ai := -1
	for i := range turn2 {
		m, _ := turn2[i].(map[string]interface{})
		if m["role"] == "assistant" {
			if _, ok := m["tool_calls"]; ok {
				ai = i
			}
		}
	}
	if ai < 0 || ai+3 >= len(turn2) {
		t.Fatalf("unexpected turn-2 message shape (len %d, assistant idx %d)", len(turn2), ai)
	}
	// Expected: assistant(tool_calls) -> tool(c1) -> tool(c2) -> user(image).
	if role(ai+1) != "tool" || role(ai+2) != "tool" {
		t.Errorf("both tool responses must immediately follow the assistant tool_calls; got %q,%q", role(ai+1), role(ai+2))
	}
	if role(ai+3) != "user" {
		t.Errorf("the image must be a user message after the tool responses; got %q", role(ai+3))
	}
}
