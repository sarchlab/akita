package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newTestReader(t *testing.T) *SQLiteTraceReader {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()

	return reader
}

func TestListExecInfo(t *testing.T) {
	reader := newTestReader(t)

	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE TABLE exec_info (Property TEXT, Value TEXT)`)
	exec(`INSERT INTO exec_info VALUES ('Command', './sim -trace')`)
	exec(`INSERT INTO exec_info VALUES ('Start Virtual Time', '0')`)
	exec(`INSERT INTO exec_info VALUES ('End Virtual Time', '6943000')`)

	entries := reader.ListExecInfo(context.Background())
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Property != "Command" || entries[0].Value != "./sim -trace" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[2].Property != "End Virtual Time" || entries[2].Value != "6943000" {
		t.Fatalf("unexpected last entry: %+v", entries[2])
	}
}

func TestListExecInfoMissingTable(t *testing.T) {
	reader := newTestReader(t)

	entries := reader.ListExecInfo(context.Background())
	if entries == nil || len(entries) != 0 {
		t.Fatalf("expected empty (non-nil) slice for missing table, got %#v", entries)
	}
}

func TestReadTopology(t *testing.T) {
	reader := newTestReader(t)

	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE TABLE component_spec (Name TEXT, Type TEXT, Spec TEXT)`)
	exec(`INSERT INTO component_spec VALUES ('L1', 'cache.Spec', '{"freq":1000}')`)
	exec(`INSERT INTO component_spec VALUES ('Agent', '', '')`)
	exec(`CREATE TABLE port (Component TEXT, Port TEXT, Connection TEXT)`)
	exec(`INSERT INTO port VALUES ('L1', 'L1.Top', 'ConnA')`)
	exec(`INSERT INTO port VALUES ('L1', 'L1.Ctrl', '')`)

	topo := reader.ReadTopology(context.Background())

	if len(topo.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(topo.Components))
	}
	l1 := topo.Components[0]
	if l1.Name != "L1" || l1.Type != "cache.Spec" {
		t.Fatalf("unexpected component: %+v", l1)
	}
	if string(l1.Spec) != `{"freq":1000}` {
		t.Fatalf("expected raw JSON spec, got %s", string(l1.Spec))
	}
	// A component without a recorded spec serializes its spec as JSON null.
	if string(topo.Components[1].Spec) != "null" {
		t.Fatalf("expected null spec for spec-less component, got %s",
			string(topo.Components[1].Spec))
	}

	if len(topo.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(topo.Ports))
	}
	// Unconnected port is present with an empty Connection.
	var ctrl *TopologyPort
	for i := range topo.Ports {
		if topo.Ports[i].Port == "L1.Ctrl" {
			ctrl = &topo.Ports[i]
		}
	}
	if ctrl == nil {
		t.Fatal("unconnected port L1.Ctrl missing from topology")
	}
	if ctrl.Connection != "" || ctrl.Component != "L1" {
		t.Fatalf("unexpected unconnected port: %+v", *ctrl)
	}
}

func TestReadTopologyMissingTables(t *testing.T) {
	reader := newTestReader(t)

	topo := reader.ReadTopology(context.Background())
	if topo.Components == nil || len(topo.Components) != 0 {
		t.Fatalf("expected empty components, got %#v", topo.Components)
	}
	if topo.Ports == nil || len(topo.Ports) != 0 {
		t.Fatalf("expected empty ports, got %#v", topo.Ports)
	}
}

func TestHTTPTopologyServesJSON(t *testing.T) {
	reader := newTestReader(t)
	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE TABLE component_spec (Name TEXT, Type TEXT, Spec TEXT)`)
	exec(`INSERT INTO component_spec VALUES ('L1', 'cache.Spec', '{"freq":1000}')`)
	exec(`CREATE TABLE port (Component TEXT, Port TEXT, Connection TEXT)`)
	exec(`INSERT INTO port VALUES ('L1', 'L1.Top', 'ConnA')`)

	s := &Server{traceReader: reader}
	rec := httptest.NewRecorder()
	s.httpTopology(rec, httptest.NewRequest(http.MethodGet, "/api/topology", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}

	var topo Topology
	if err := json.Unmarshal(rec.Body.Bytes(), &topo); err != nil {
		t.Fatalf("unmarshal body: %v (body=%s)", err, rec.Body.String())
	}
	if len(topo.Components) != 1 || topo.Components[0].Name != "L1" {
		t.Fatalf("unexpected components: %+v", topo.Components)
	}
	// Spec round-trips as a real JSON object, not a quoted string.
	var spec struct {
		Freq int `json:"freq"`
	}
	if err := json.Unmarshal(topo.Components[0].Spec, &spec); err != nil {
		t.Fatalf("spec is not embedded JSON: %v", err)
	}
	if spec.Freq != 1000 {
		t.Fatalf("unexpected spec.freq: %d", spec.Freq)
	}
	if len(topo.Ports) != 1 || topo.Ports[0].Connection != "ConnA" {
		t.Fatalf("unexpected ports: %+v", topo.Ports)
	}
}

func TestHTTPSimInfoServesJSON(t *testing.T) {
	reader := newTestReader(t)
	if _, err := reader.Exec(
		`CREATE TABLE exec_info (Property TEXT, Value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Exec(
		`INSERT INTO exec_info VALUES ('Command', './sim')`); err != nil {
		t.Fatal(err)
	}

	s := &Server{traceReader: reader}
	rec := httptest.NewRecorder()
	s.httpSimInfo(rec, httptest.NewRequest(http.MethodGet, "/api/sim_info", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var entries []SimInfoEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 || entries[0].Property != "Command" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}
