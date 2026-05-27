package monitoring2

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

type fieldValueResponseNode struct {
	K int             `json:"k"`
	T string          `json:"t"`
	V json.RawMessage `json:"v"`
	L *int            `json:"l"`
	O *int            `json:"o"`
}

type fieldValueResponse struct {
	R    string                            `json:"r"`
	Dict map[string]fieldValueResponseNode `json:"dict"`
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
	monitor := newSliceFieldMonitor([]int{10, 20, 30, 40, 50})
	recorder := requestSliceFieldPage(t, monitor, 2, 2)
	response := decodeFieldValueResponse(t, recorder)
	ids := assertSlicePageRoot(t, response, 5, 2, 2)

	assertSlicePageValues(t, response, ids, []int{30, 40})
}

func newSliceFieldMonitor(values []int) *Monitor {
	monitor := NewMonitor()
	monitor.RegisterEngine(&fakeEngine{})
	monitor.RegisterComponent(newSliceFieldComponent("slice-comp", values))

	return monitor
}

func requestSliceFieldPage(
	t *testing.T,
	monitor *Monitor,
	offset, limit int,
) *httptest.ResponseRecorder {
	t.Helper()

	requestJSON := `{"comp_name":"slice-comp","field_name":"State.Values"}`
	requestPath := "/api/field/" + url.PathEscape(requestJSON) +
		"?slice_offset=" + strconv.Itoa(offset) +
		"&slice_limit=" + strconv.Itoa(limit)
	request := httptest.NewRequest(http.MethodGet, requestPath, nil)
	recorder := httptest.NewRecorder()

	monitor.listFieldValue(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	return recorder
}

func decodeFieldValueResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) fieldValueResponse {
	t.Helper()

	var response fieldValueResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	return response
}

func assertSlicePageRoot(
	t *testing.T,
	response fieldValueResponse,
	length, offset, visible int,
) []string {
	t.Helper()

	root := response.Dict[response.R]
	if root.L == nil || *root.L != length {
		t.Fatalf("expected root length %d, got %#v", length, root.L)
	}

	if root.O == nil || *root.O != offset {
		t.Fatalf("expected root offset %d, got %#v", offset, root.O)
	}

	var ids []string
	if err := json.Unmarshal(root.V, &ids); err != nil {
		t.Fatal(err)
	}

	if len(ids) != visible {
		t.Fatalf("expected %d visible IDs, got %d", visible, len(ids))
	}

	if len(response.Dict) != visible+1 {
		t.Fatalf("expected root plus %d values, got %d nodes",
			visible, len(response.Dict))
	}

	return ids
}

func assertSlicePageValues(
	t *testing.T,
	response fieldValueResponse,
	ids []string,
	expected []int,
) {
	t.Helper()

	for i, id := range ids {
		var value int
		if err := json.Unmarshal(response.Dict[id].V, &value); err != nil {
			t.Fatal(err)
		}

		if value != expected[i] {
			t.Fatalf("expected value %d at index %d, got %d",
				expected[i], i, value)
		}
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
