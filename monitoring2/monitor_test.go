package monitoring2

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

type fakeEngine struct {
	hooking.HookableBase

	now           timing.VTimeInSec
	pauseCalls    int
	continueCalls int
}

func (e *fakeEngine) Schedule(timing.Event) {}

func (e *fakeEngine) Run() error {
	return nil
}

func (e *fakeEngine) Pause() {
	e.pauseCalls++
}

func (e *fakeEngine) Continue() {
	e.continueCalls++
}

func (e *fakeEngine) CurrentTime() timing.VTimeInSec {
	return e.now
}

type sliceFieldState struct {
	Values []int
}

type sliceFieldComponent struct {
	hooking.HookableBase
	*messaging.PortOwnerBase

	State sliceFieldState
	name  string
}

func newSliceFieldComponent(name string, values []int) *sliceFieldComponent {
	return &sliceFieldComponent{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		State:         sliceFieldState{Values: values},
		name:          name,
	}
}

func (c *sliceFieldComponent) Name() string {
	return c.name
}

func (c *sliceFieldComponent) NotifyRecv(messaging.Port) {}

func (c *sliceFieldComponent) NotifyPortFree(messaging.Port) {}

func TestEngineStateTracksPauseContinueIdempotently(t *testing.T) {
	engine := &fakeEngine{}
	monitor := NewMonitor()
	monitor.RegisterEngine(engine)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/engine/state", nil)
	monitor.apiEngineState(recorder, request)

	var response engineStateRsp
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.State != "running" || response.Paused {
		t.Fatalf("expected running state, got %#v", response)
	}

	for i := 0; i < 2; i++ {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, "/api/pause", nil)
		monitor.pauseEngine(recorder, request)
	}

	if engine.pauseCalls != 1 {
		t.Fatalf("expected one engine pause call, got %d", engine.pauseCalls)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/engine/state", nil)
	monitor.apiEngineState(recorder, request)

	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.State != "paused" || !response.Paused {
		t.Fatalf("expected paused state, got %#v", response)
	}

	for i := 0; i < 2; i++ {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, "/api/continue", nil)
		monitor.continueEngine(recorder, request)
	}

	if engine.continueCalls != 1 {
		t.Fatalf("expected one engine continue call, got %d", engine.continueCalls)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/engine/state", nil)
	monitor.apiEngineState(recorder, request)

	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.State != "running" || response.Paused {
		t.Fatalf("expected running state after continue, got %#v", response)
	}
}

func TestFieldValuePaginatesSlice(t *testing.T) {
	engine := &fakeEngine{}
	monitor := NewMonitor()
	monitor.RegisterEngine(engine)
	monitor.RegisterComponent(newSliceFieldComponent(
		"slice-comp",
		[]int{10, 20, 30, 40, 50},
	))

	requestJSON := `{"comp_name":"slice-comp","field_name":"State.Values"}`
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/field/"+url.PathEscape(requestJSON)+
			"?slice_offset=2&slice_limit=2",
		nil,
	)
	recorder := httptest.NewRecorder()

	monitor.listFieldValue(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	type responseNode struct {
		K int             `json:"k"`
		T string          `json:"t"`
		V json.RawMessage `json:"v"`
		L *int            `json:"l"`
		O *int            `json:"o"`
	}

	var response struct {
		R    string                  `json:"r"`
		Dict map[string]responseNode `json:"dict"`
	}

	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	root := response.Dict[response.R]
	if root.L == nil || *root.L != 5 {
		t.Fatalf("expected root length 5, got %#v", root.L)
	}

	if root.O == nil || *root.O != 2 {
		t.Fatalf("expected root offset 2, got %#v", root.O)
	}

	var ids []string
	if err := json.Unmarshal(root.V, &ids); err != nil {
		t.Fatal(err)
	}

	if len(ids) != 2 {
		t.Fatalf("expected 2 visible IDs, got %d", len(ids))
	}

	if len(response.Dict) != 3 {
		t.Fatalf("expected root plus 2 values, got %d nodes", len(response.Dict))
	}

	var firstValue int
	if err := json.Unmarshal(response.Dict[ids[0]].V, &firstValue); err != nil {
		t.Fatal(err)
	}

	var secondValue int
	if err := json.Unmarshal(response.Dict[ids[1]].V, &secondValue); err != nil {
		t.Fatal(err)
	}

	if firstValue != 30 || secondValue != 40 {
		t.Fatalf("expected values 30 and 40, got %d and %d",
			firstValue, secondValue)
	}
}

func TestExecutionInfoReadsExecInfoTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "trace.sqlite3")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE exec_info (Property TEXT, Value TEXT)`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO exec_info (Property, Value) VALUES (?, ?)`,
		"Command", "akita")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	monitor := NewMonitor()
	monitor.SetTraceDBPath(dbPath)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/execution/info", nil)

	monitor.apiExecutionInfo(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response []executionInfoEntry
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if len(response) != 1 {
		t.Fatalf("expected 1 execution info entry, got %d", len(response))
	}

	if response[0].Property != "Command" || response[0].Value != "akita" {
		t.Fatalf("unexpected execution info entry: %#v", response[0])
	}
}

func TestTraceStorageReportsDatabaseAndDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "trace.sqlite3")

	if err := os.WriteFile(dbPath, make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(dbPath+"-wal", make([]byte, 5), 0o600); err != nil {
		t.Fatal(err)
	}

	monitor := NewMonitor()
	monitor.SetTraceDBPath(dbPath)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/trace/storage", nil)

	monitor.apiTraceStorage(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response traceStorageRsp
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.Path != dbPath {
		t.Fatalf("expected path %q, got %q", dbPath, response.Path)
	}

	if response.FileSizeBytes != 7 {
		t.Fatalf("expected file size 7, got %d", response.FileSizeBytes)
	}

	if response.SidecarSizeBytes != 5 {
		t.Fatalf("expected sidecar size 5, got %d", response.SidecarSizeBytes)
	}

	if response.TotalSizeBytes != 12 {
		t.Fatalf("expected total size 12, got %d", response.TotalSizeBytes)
	}

	if response.DiskAvailableBytes == 0 {
		t.Fatal("expected available disk bytes")
	}

	if response.DiskTotalBytes == 0 {
		t.Fatal("expected total disk bytes")
	}
}
