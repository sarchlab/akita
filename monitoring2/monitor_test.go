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
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type fakeEngine struct {
	hooking.HookableBase

	mu            sync.Mutex
	now           timing.VTimeInSec
	pauseCalls    int
	continueCalls int
	runCalls      int
	runReady      chan struct{}
}

func (e *fakeEngine) Schedule(timing.Event) {}

func (e *fakeEngine) Run() error {
	e.mu.Lock()
	e.runCalls++
	ready := e.runReady
	e.mu.Unlock()

	if ready != nil {
		close(ready)
	}

	return nil
}

func (e *fakeEngine) Pause() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pauseCalls++
}

func (e *fakeEngine) Continue() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.continueCalls++
}

func (e *fakeEngine) CurrentTime() timing.VTimeInSec {
	e.mu.Lock()
	defer e.mu.Unlock()

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

func TestApiModeReturnsLiveJSON(t *testing.T) {
	monitor := NewMonitor()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/mode", nil)
	monitor.apiMode(recorder, request)

	var response struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.Mode != "live" {
		t.Fatalf("expected mode %q, got %q", "live", response.Mode)
	}
}

func TestNowReportsEngineCurrentTime(t *testing.T) {
	engine := &fakeEngine{now: timing.VTimeInSec(1234)}
	monitor := NewMonitor()
	monitor.RegisterEngine(engine)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/now", nil)
	monitor.now(recorder, request)

	var response struct {
		Now timing.VTimeInSec `json:"now"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.Now != 1234 {
		t.Fatalf("expected now=1234, got %v", response.Now)
	}
}

func TestRunInvokesEngineRun(t *testing.T) {
	engine := &fakeEngine{runReady: make(chan struct{})}
	monitor := NewMonitor()
	monitor.RegisterEngine(engine)

	monitor.run(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/api/run", nil))

	<-engine.runReady

	engine.mu.Lock()
	defer engine.mu.Unlock()

	if engine.runCalls != 1 {
		t.Fatalf("expected one engine run call, got %d", engine.runCalls)
	}
}

func TestListComponentsReturnsRegisteredNames(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterComponent(newSliceFieldComponent("alpha", nil))
	monitor.RegisterComponent(newSliceFieldComponent("beta", nil))

	recorder := httptest.NewRecorder()
	monitor.listComponents(recorder,
		httptest.NewRequest(http.MethodGet, "/api/list_components", nil))

	var names []string
	if err := json.NewDecoder(recorder.Body).Decode(&names); err != nil {
		t.Fatal(err)
	}

	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("unexpected names: %#v", names)
	}
}

func TestListComponentDetailsReturns404ForUnknown(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterEngine(&fakeEngine{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/api/component/missing", nil)

	monitor.listComponentDetails(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
}

func TestListComponentDetailsSerializesRegisteredComponent(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterEngine(&fakeEngine{})
	monitor.RegisterComponent(newSliceFieldComponent("slice-comp", []int{1, 2}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/api/component/slice-comp", nil)

	monitor.listComponentDetails(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d",
			http.StatusOK, recorder.Code)
	}

	var payload struct {
		Root string `json:"r"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got %v", err)
	}

	if payload.Root == "" {
		t.Fatalf("expected non-empty root id, got %#v", payload)
	}
}

type tickableComponent struct {
	hooking.HookableBase
	*messaging.PortOwnerBase

	name      string
	tickCalls int
}

func newTickableComponent(name string) *tickableComponent {
	return &tickableComponent{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		name:          name,
	}
}

func (c *tickableComponent) Name() string                  { return c.name }
func (c *tickableComponent) NotifyRecv(messaging.Port)     {}
func (c *tickableComponent) NotifyPortFree(messaging.Port) {}
func (c *tickableComponent) TickLater()                    { c.tickCalls++ }

func TestTickInvokesTickLaterOnTickingComponent(t *testing.T) {
	monitor := NewMonitor()
	tickable := newTickableComponent("ticker")
	monitor.RegisterComponent(tickable)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/tick/ticker", nil)

	monitor.tick(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	if tickable.tickCalls != 1 {
		t.Fatalf("expected one TickLater call, got %d", tickable.tickCalls)
	}
}

func TestTickReturns405ForNonTickingComponent(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterComponent(newSliceFieldComponent("slice-comp", nil))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/tick/slice-comp", nil)

	monitor.tick(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}
}

func TestTickReturns404ForUnknownComponent(t *testing.T) {
	monitor := NewMonitor()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/tick/missing", nil)

	monitor.tick(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
}

func TestProgressBarsLifecycleRoundtripsThroughHandler(t *testing.T) {
	monitor := NewMonitor()

	requireEmpty := func() {
		recorder := httptest.NewRecorder()
		monitor.listProgressBars(recorder,
			httptest.NewRequest(http.MethodGet, "/api/progress", nil))

		if got := recorder.Body.String(); got != "[]" {
			t.Fatalf("expected empty array, got %q", got)
		}
	}

	requireEmpty()

	bar := monitor.CreateProgressBar("decode", 100)

	recorder := httptest.NewRecorder()
	monitor.listProgressBars(recorder,
		httptest.NewRequest(http.MethodGet, "/api/progress", nil))

	var bars []struct {
		Name  string `json:"name"`
		Total uint64 `json:"total"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&bars); err != nil {
		t.Fatal(err)
	}

	if len(bars) != 1 || bars[0].Name != "decode" || bars[0].Total != 100 {
		t.Fatalf("unexpected progress bar list: %#v", bars)
	}

	monitor.CompleteProgressBar(bar)
	requireEmpty()
}

type bufferOnlyComponent struct {
	hooking.HookableBase
	*messaging.PortOwnerBase

	Buf  queueing.Buffer[int]
	name string
}

func newBufferOnlyComponent(
	name string, capacity, filled int,
) *bufferOnlyComponent {
	c := &bufferOnlyComponent{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		Buf:           queueing.NewBuffer[int](name+".buf", capacity),
		name:          name,
	}

	for i := 0; i < filled; i++ {
		c.Buf.PushTyped(i)
	}

	return c
}

func (c *bufferOnlyComponent) Name() string                  { return c.name }
func (c *bufferOnlyComponent) NotifyRecv(messaging.Port)     {}
func (c *bufferOnlyComponent) NotifyPortFree(messaging.Port) {}

type portedComponent struct {
	hooking.HookableBase
	*messaging.PortOwnerBase

	name string
}

func newPortedComponent(name string) *portedComponent {
	c := &portedComponent{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		name:          name,
	}
	c.AddPort("p", messaging.NewPort(c, 4, 4, name+".p"))

	return c
}

func (c *portedComponent) Name() string                  { return c.name }
func (c *portedComponent) NotifyRecv(messaging.Port)     {}
func (c *portedComponent) NotifyPortFree(messaging.Port) {}

type bufferRsp struct {
	Buffer string `json:"buffer"`
	Level  int    `json:"level"`
	Cap    int    `json:"cap"`
}

func TestHangDetectorBuffersSortsByPercentByDefault(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterComponent(newBufferOnlyComponent("low", 10, 1))
	monitor.RegisterComponent(newBufferOnlyComponent("high", 4, 3))
	monitor.RegisterComponent(newBufferOnlyComponent("mid", 4, 2))

	recorder := httptest.NewRecorder()
	monitor.hangDetectorBuffers(recorder,
		httptest.NewRequest(http.MethodGet,
			"/api/hangdetector/buffers?limit=3", nil))

	var bufs []bufferRsp
	if err := json.NewDecoder(recorder.Body).Decode(&bufs); err != nil {
		t.Fatal(err)
	}

	if len(bufs) != 3 {
		t.Fatalf("expected 3 buffers, got %d", len(bufs))
	}

	if bufs[0].Buffer != "high.buf" || bufs[1].Buffer != "mid.buf" ||
		bufs[2].Buffer != "low.buf" {
		t.Fatalf("unexpected percent sort: %#v", bufs)
	}
}

func TestHangDetectorBuffersSortsByLevelHonorsPagination(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterComponent(newBufferOnlyComponent("a", 10, 5))
	monitor.RegisterComponent(newBufferOnlyComponent("b", 10, 7))
	monitor.RegisterComponent(newBufferOnlyComponent("c", 10, 3))

	recorder := httptest.NewRecorder()
	monitor.hangDetectorBuffers(recorder,
		httptest.NewRequest(http.MethodGet,
			"/api/hangdetector/buffers?sort=level&limit=2&offset=1", nil))

	var bufs []bufferRsp
	if err := json.NewDecoder(recorder.Body).Decode(&bufs); err != nil {
		t.Fatal(err)
	}

	if len(bufs) != 2 {
		t.Fatalf("expected 2 buffers, got %d", len(bufs))
	}

	if bufs[0].Buffer != "a.buf" || bufs[0].Level != 5 ||
		bufs[1].Buffer != "c.buf" || bufs[1].Level != 3 {
		t.Fatalf("unexpected level page: %#v", bufs)
	}
}

func TestHangDetectorBuffersIncludesPortAdapters(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterComponent(newPortedComponent("comp"))

	recorder := httptest.NewRecorder()
	monitor.hangDetectorBuffers(recorder,
		httptest.NewRequest(http.MethodGet,
			"/api/hangdetector/buffers?sort=level&limit=10", nil))

	var bufs []bufferRsp
	if err := json.NewDecoder(recorder.Body).Decode(&bufs); err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, b := range bufs {
		names[b.Buffer] = true
	}

	if !names["comp.p.in"] || !names["comp.p.out"] {
		t.Fatalf("expected port adapters in result, got %#v", bufs)
	}
}

func TestHangDetectorBuffersRejectsInvalidSort(t *testing.T) {
	monitor := NewMonitor()

	recorder := httptest.NewRecorder()
	monitor.hangDetectorBuffers(recorder,
		httptest.NewRequest(http.MethodGet,
			"/api/hangdetector/buffers?sort=bogus", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func newTestDBTracer(t *testing.T) *tracing.DBTracer {
	t.Helper()

	recorder := datarecording.NewDataRecorder(
		filepath.Join(t.TempDir(), "tracer"))
	t.Cleanup(func() {
		_ = recorder.Close()
	})

	return tracing.NewDBTracer(&fakeEngine{}, recorder)
}

func TestTraceLifecycleRoundtripsStartEndStatus(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterVisTracer(newTestDBTracer(t))

	assertTracing := func(expected bool) {
		t.Helper()

		recorder := httptest.NewRecorder()
		monitor.apiTraceIsTracing(recorder,
			httptest.NewRequest(http.MethodGet, "/api/trace/is_tracing", nil))

		var response struct {
			IsTracing bool `json:"isTracing"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.IsTracing != expected {
			t.Fatalf("expected isTracing=%v, got %v",
				expected, response.IsTracing)
		}
	}

	assertTracing(false)

	startRecorder := httptest.NewRecorder()
	monitor.apiTraceStart(startRecorder,
		httptest.NewRequest(http.MethodPost, "/api/trace/start", nil))
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("expected start 200, got %d", startRecorder.Code)
	}

	assertTracing(true)

	endRecorder := httptest.NewRecorder()
	monitor.apiTraceEnd(endRecorder,
		httptest.NewRequest(http.MethodPost, "/api/trace/end", nil))
	if endRecorder.Code != http.StatusOK {
		t.Fatalf("expected end 200, got %d", endRecorder.Code)
	}

	assertTracing(false)
}

func TestTraceStartEndRequirePOST(t *testing.T) {
	monitor := NewMonitor()
	monitor.RegisterVisTracer(newTestDBTracer(t))

	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/api/trace/start", monitor.apiTraceStart},
		{"/api/trace/end", monitor.apiTraceEnd},
	} {
		recorder := httptest.NewRecorder()
		tc.handler(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))

		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 from GET %s, got %d",
				tc.path, recorder.Code)
		}
	}
}

func TestTraceHandlersWhenTracerUnset(t *testing.T) {
	monitor := NewMonitor()

	recorder := httptest.NewRecorder()
	monitor.apiTraceIsTracing(recorder,
		httptest.NewRequest(http.MethodGet, "/api/trace/is_tracing", nil))

	var status struct {
		IsTracing bool `json:"isTracing"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}

	if status.IsTracing {
		t.Fatalf("expected isTracing=false when tracer unset, got true")
	}

	startRecorder := httptest.NewRecorder()
	monitor.apiTraceStart(startRecorder,
		httptest.NewRequest(http.MethodPost, "/api/trace/start", nil))

	if startRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when tracer unset, got %d",
			startRecorder.Code)
	}

	endRecorder := httptest.NewRecorder()
	monitor.apiTraceEnd(endRecorder,
		httptest.NewRequest(http.MethodPost, "/api/trace/end", nil))

	if endRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when tracer unset, got %d",
			endRecorder.Code)
	}
}
