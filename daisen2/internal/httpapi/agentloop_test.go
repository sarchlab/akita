package httpapi

import (
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

	if got, _ := sanitizeReadonlySQL("SELECT * FROM trace LIMIT 5", 100); strings.Count(strings.ToUpper(got), "LIMIT") != 1 {
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
		"SELECT loc.Locale, COUNT(*) AS n FROM trace t JOIN location loc ON t.Location = loc.ID GROUP BY loc.Locale ORDER BY loc.Locale")
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

// TestRunAgentLoop_MockProvider is the end-to-end "preview": a scripted mock LLM
// drives the real loop against a real trace DB. Turn 1 asks for a data_query;
// the loop runs the SQL; turn 2 returns the final answer from the observation.
func TestRunAgentLoop_MockProvider(t *testing.T) {
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
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[`+
				`{"id":"c1","type":"function","function":{"name":"data_query",`+
				`"arguments":"{\"sql\":\"SELECT loc.Locale, COUNT(*) AS n FROM trace t JOIN location loc ON t.Location=loc.ID GROUP BY loc.Locale\"}"}}]}}]}`)
			return
		}
		// Second turn: the conversation now carries the tool result.
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"L2Cache handled 2 tasks; L1Cache handled 1."}}]}`)
	}))
	defer srv.Close()

	cfg := ProviderConfig{Provider: ProviderOpenAICompatible, BaseURL: srv.URL, Model: "mock"}
	messages := []map[string]interface{}{{"role": "user", "content": "How many tasks per component?"}}

	var events []agentEvent
	emit := func(ev agentEvent) { events = append(events, ev) }

	if err := runAgentLoop(context.Background(), cfg, messages, []agentTool{dataQueryTool(reader)}, emit); err != nil {
		t.Fatalf("runAgentLoop: %v", err)
	}

	var sawStep, sawObs, sawMsg bool
	for _, ev := range events {
		switch ev.Type {
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
