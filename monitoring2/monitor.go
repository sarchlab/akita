package monitoring2

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	// Enable profiling.
	_ "github.com/glebarez/go-sqlite"
	_ "net/http/pprof"

	"github.com/google/pprof/profile"
	"github.com/sarchlab/akita/v5/daisen2"
	"github.com/sarchlab/akita/v5/monitoring2/static"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/shirou/gopsutil/process"
	"github.com/syifan/goseth"

	// Monitor provides live simulation monitoring capabilities. It serves HTTP
	// endpoints for engine control, component inspection, progress bars, and
	// resource monitoring.
	"github.com/sarchlab/akita/v5/messaging"
)

type Monitor struct {
	// Configuration (set before StartServer).
	port      int
	engine    timing.Engine
	visTracer *tracing.DBTracer
	tracePath string

	// Internal state.
	components       []messaging.Component
	buffers          []bufferState
	engineControlMu  sync.Mutex
	enginePaused     bool
	progressBarsLock sync.Mutex
	progressBars     []*daisen2.ProgressBar
	httpServer       *http.Server
	fs               http.FileSystem
}

// NewMonitor creates a new Monitor with default settings. The monitor is not
// started until StartServer() is called.
func NewMonitor() *Monitor {
	return &Monitor{
		fs:           static.GetAssets(),
		progressBars: []*daisen2.ProgressBar{},
	}
}

// WithPortNumber sets the port number for the monitoring server. Returns the
// Monitor for method chaining.
func (m *Monitor) WithPortNumber(port int) *Monitor {
	if port < 1000 {
		fmt.Fprintf(os.Stderr,
			"Port number %d is assigned to the server, "+
				"which is not allowed. Using a random port instead.\n",
			port,
		)
		port = 0
	}

	m.port = port

	return m
}

// RegisterEngine registers the simulation engine with the monitor.
func (m *Monitor) RegisterEngine(e timing.Engine) {
	m.engine = e
}

// RegisterComponent registers a component with the monitor so its internal
// state can be inspected via the monitoring server.
func (m *Monitor) RegisterComponent(c messaging.Component) {
	m.components = append(m.components, c)
	m.registerBuffers(c)
}

// RegisterVisTracer registers a visualization tracer with the monitor.
func (m *Monitor) RegisterVisTracer(tr *tracing.DBTracer) {
	m.visTracer = tr
}

// SetTraceDBPath sets the SQLite trace database path used for storage status.
func (m *Monitor) SetTraceDBPath(path string) {
	m.tracePath = path
}

// GetServer is kept for compatibility with older monitoring setup code.
// Monitoring2 no longer owns a Daisen replay server.
func (m *Monitor) GetServer() *daisen2.Server {
	return nil
}

// CreateProgressBar creates a new progress bar tracked by the monitor.
func (m *Monitor) CreateProgressBar(name string, total uint64) *daisen2.ProgressBar {
	bar := &daisen2.ProgressBar{
		ID:    timing.GetIDGenerator().Generate(),
		Name:  name,
		Total: total,
	}

	m.progressBarsLock.Lock()
	defer m.progressBarsLock.Unlock()

	m.progressBars = append(m.progressBars, bar)

	return bar
}

// CompleteProgressBar removes a bar from the progress list.
func (m *Monitor) CompleteProgressBar(pb *daisen2.ProgressBar) {
	m.progressBarsLock.Lock()
	defer m.progressBarsLock.Unlock()

	newBars := make([]*daisen2.ProgressBar, 0, len(m.progressBars)-1)

	for _, b := range m.progressBars {
		if b != pb {
			newBars = append(newBars, b)
		}
	}

	m.progressBars = newBars
}

// StartServer initializes and starts the monitoring HTTP server.
func (m *Monitor) StartServer() {
	// Build combined mux.
	mux := http.NewServeMux()

	// Register live-mode endpoints.
	mux.HandleFunc("/api/mode", m.apiMode)
	mux.HandleFunc("/api/pause", m.pauseEngine)
	mux.HandleFunc("/api/continue", m.continueEngine)
	mux.HandleFunc("/api/engine/state", m.apiEngineState)
	mux.HandleFunc("/api/now", m.now)
	mux.HandleFunc("/api/run", m.run)
	mux.HandleFunc("/api/tick/", m.tick)
	mux.HandleFunc("/api/list_components", m.listComponents)
	mux.HandleFunc("/api/component/", m.listComponentDetails)
	mux.HandleFunc("/api/field/", m.listFieldValue)
	mux.HandleFunc("/api/hangdetector/buffers", m.hangDetectorBuffers)
	mux.HandleFunc("/api/progress", m.listProgressBars)
	mux.HandleFunc("/api/execution/info", m.apiExecutionInfo)
	mux.HandleFunc("/api/resource", m.listResources)
	mux.HandleFunc("/api/profile", m.collectProfile)
	mux.HandleFunc("/api/trace/start", m.apiTraceStart)
	mux.HandleFunc("/api/trace/end", m.apiTraceEnd)
	mux.HandleFunc("/api/trace/is_tracing", m.apiTraceIsTracing)
	mux.HandleFunc("/api/trace/storage", m.apiTraceStorage)

	m.setupStaticRoutes(mux)

	// Find port and start listener.
	listener := m.findPort()
	port := listener.Addr().(*net.TCPAddr).Port

	fmt.Fprintf(os.Stderr,
		"Monitoring simulation with http://localhost:%d\n", port)

	m.httpServer = &http.Server{Handler: mux}

	go func() {
		err := m.httpServer.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			log.Panic(err)
		}
	}()
}

func (m *Monitor) setupStaticRoutes(mux *http.ServeMux) {
	fServer := http.FileServer(m.fs)
	mux.HandleFunc("/dashboard", m.serveIndex)
	mux.HandleFunc("/component", m.serveIndex)
	mux.HandleFunc("/task", m.serveIndex)
	mux.HandleFunc("/execution", m.serveIndex)
	mux.HandleFunc("/progress", m.serveIndex)
	mux.HandleFunc("/monitor", m.serveIndex)
	mux.HandleFunc("/analysis", m.serveIndex)
	mux.HandleFunc("/debug", m.serveIndex)
	mux.HandleFunc("/profiling", m.serveIndex)
	mux.HandleFunc("/live", m.serveIndex)
	mux.HandleFunc("/live/", m.serveIndex)
	mux.Handle("/", fServer)
}

func (m *Monitor) findPort() net.Listener {
	if m.port > 0 {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", m.port))
		if err != nil {
			log.Panicf("failed to listen on port %d: %v", m.port, err)
		}

		return listener
	}

	startPort := 32776
	maxAttempts := 100

	for i := 0; i < maxAttempts; i++ {
		listener, err := net.Listen("tcp",
			fmt.Sprintf(":%d", startPort+i))
		if err == nil {
			return listener
		}
	}

	log.Panic("failed to find available port")

	return nil
}

// StopServer gracefully shuts down the monitoring server.
func (m *Monitor) StopServer() {
	if m.httpServer != nil {
		m.httpServer.Close()
	}
}

// ---- Buffer inspection ----

// bufferState is a minimal interface for buffer inspection by the hang detector.
type bufferState interface {
	Name() string
	Size() int
	Capacity() int
}

// portBufferAdapter wraps a port to expose one of its internal buffers
// (incoming or outgoing) as a bufferState for the hang detector.
type portBufferAdapter struct {
	port      messaging.Port
	direction string // "in" or "out"
}

func (a *portBufferAdapter) Name() string {
	return a.port.Name() + "." + a.direction
}

func (a *portBufferAdapter) Size() int {
	if a.direction == "in" {
		return a.port.NumIncoming()
	}

	return a.port.NumOutgoing()
}

func (a *portBufferAdapter) Capacity() int {
	return a.Size()
}

func (m *Monitor) registerBuffers(c messaging.Component) {
	m.registerComponentOrPortBuffers(c)

	for _, p := range c.Ports() {
		m.registerComponentOrPortBuffers(p)
		m.registerPortBuffers(p)
	}
}

func (m *Monitor) registerPortBuffers(p messaging.Port) {
	m.buffers = append(m.buffers,
		&portBufferAdapter{port: p, direction: "in"},
		&portBufferAdapter{port: p, direction: "out"},
	)
}

func (m *Monitor) registerComponentOrPortBuffers(c any) {
	v := reflect.ValueOf(c).Elem()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := field.Type()
		bufferType := reflect.TypeOf((*bufferState)(nil)).Elem()

		if fieldType.Implements(bufferType) ||
			reflect.PointerTo(fieldType).Implements(bufferType) {
			fieldRef := reflect.NewAt(
				field.Type(),
				unsafe.Pointer(field.UnsafeAddr()),
			).Elem().Interface().(bufferState)
			m.buffers = append(m.buffers, fieldRef)
		}
	}
}

// ---- Live HTTP handlers ----

func (m *Monitor) apiMode(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"mode":"live"}`)
}

func (m *Monitor) serveIndex(w http.ResponseWriter, _ *http.Request) {
	f, err := m.fs.Open("index.html")
	if err != nil {
		log.Panic(err)
	}

	p, err := readAll(f)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(p)
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) pauseEngine(w http.ResponseWriter, _ *http.Request) {
	m.engineControlMu.Lock()
	if !m.enginePaused {
		m.engine.Pause()
		m.enginePaused = true
	}
	response := m.engineStateResponseLocked()
	m.engineControlMu.Unlock()

	m.writeEngineState(w, response)
}

func (m *Monitor) continueEngine(w http.ResponseWriter, _ *http.Request) {
	m.engineControlMu.Lock()
	if m.enginePaused {
		m.engine.Continue()
		m.enginePaused = false
	}
	response := m.engineStateResponseLocked()
	m.engineControlMu.Unlock()

	m.writeEngineState(w, response)
}

type engineStateRsp struct {
	State  string `json:"state"`
	Paused bool   `json:"paused"`
}

func (m *Monitor) apiEngineState(w http.ResponseWriter, _ *http.Request) {
	m.engineControlMu.Lock()
	response := m.engineStateResponseLocked()
	m.engineControlMu.Unlock()

	m.writeEngineState(w, response)
}

func (m *Monitor) engineStateResponseLocked() engineStateRsp {
	if m.enginePaused {
		return engineStateRsp{State: "paused", Paused: true}
	}

	return engineStateRsp{State: "running", Paused: false}
}

func (m *Monitor) writeEngineState(w http.ResponseWriter, response engineStateRsp) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
	}
}

func (m *Monitor) now(w http.ResponseWriter, _ *http.Request) {
	nowTime := m.engine.CurrentTime()
	fmt.Fprintf(w, "{\"now\":%d}", nowTime)
}

func (m *Monitor) pauseForInspection() func() {
	m.engineControlMu.Lock()

	if m.enginePaused {
		return func() {
			m.engineControlMu.Unlock()
		}
	}

	m.engine.Pause()

	return func() {
		m.engine.Continue()
		m.engineControlMu.Unlock()
	}
}

func (m *Monitor) run(_ http.ResponseWriter, _ *http.Request) {
	go func() {
		err := m.engine.Run()
		if err != nil {
			panic(err)
		}
	}()
}

func (m *Monitor) listComponents(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprint(w, "[")

	for i, c := range m.components {
		if i > 0 {
			fmt.Fprint(w, ",")
		}

		fmt.Fprintf(w, "\"%s\"", c.Name())
	}

	fmt.Fprint(w, "]")
}

type tickingComponent interface {
	TickLater()
}

func (m *Monitor) tick(w http.ResponseWriter, r *http.Request) {
	compName := strings.TrimPrefix(r.URL.Path, "/api/tick/")

	comp := m.findComponentOr404(w, compName)
	if comp == nil {
		return
	}

	tickingComp, ok := comp.(tickingComponent)
	if !ok {
		w.WriteHeader(405)
		return
	}

	tickingComp.TickLater()
	w.WriteHeader(200)
}

func (m *Monitor) listComponentDetails(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := strings.TrimPrefix(r.URL.Path, "/api/component/")

	component := m.findComponentOr404(w, name)
	if component == nil {
		return
	}

	resume := m.pauseForInspection()
	defer resume()

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)

	err := serializer.Serialize(w)
	if err != nil {
		log.Panic(err)
	}
}

type fieldReq struct {
	CompName  string `json:"comp_name,omitempty"`
	FieldName string `json:"field_name,omitempty"`
}

const (
	defaultSlicePageLimit = 50
	maxSlicePageLimit     = 1000
)

func (m *Monitor) listFieldValue(w http.ResponseWriter, r *http.Request) {
	jsonString := strings.TrimPrefix(r.URL.Path, "/api/field/")
	req := fieldReq{}

	err := json.Unmarshal([]byte(jsonString), &req)
	if err != nil {
		log.Panic(err)
	}

	name := req.CompName
	fields := strings.Split(req.FieldName, ".")

	component := m.findComponentOr404(w, name)
	if component == nil {
		return
	}

	resume := m.pauseForInspection()
	defer resume()

	sliceOffset, sliceLimit, pagingRequested, err := parseSlicePageParams(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error: %s", err)

		return
	}

	if pagingRequested {
		value, err := monitorEntryPointValue(component, fields)
		if err != nil {
			log.Panic(err)
		}

		if monitorStrip(value).Kind() == reflect.Slice {
			err = writeSlicePage(w, value, sliceOffset, sliceLimit)
			if err != nil {
				log.Panic(err)
			}

			return
		}
	}

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)

	err = serializer.SetEntryPoint(fields)
	if err != nil {
		log.Panic(err)
	}

	err = serializer.Serialize(w)
	if err != nil {
		log.Panic(err)
	}
}

func parseSlicePageParams(
	r *http.Request,
) (offset, limit int, requested bool, err error) {
	query := r.URL.Query()
	offsetStr := query.Get("slice_offset")
	limitStr := query.Get("slice_limit")

	if offsetStr == "" && limitStr == "" {
		return 0, 0, false, nil
	}

	limit = defaultSlicePageLimit

	if offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)
		if err != nil {
			return 0, 0, true, fmt.Errorf("invalid slice_offset %q", offsetStr)
		}
	}

	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			return 0, 0, true, fmt.Errorf("invalid slice_limit %q", limitStr)
		}
	}

	if offset < 0 {
		return 0, 0, true, fmt.Errorf("slice_offset must be non-negative")
	}

	if limit <= 0 {
		return 0, 0, true, fmt.Errorf("slice_limit must be positive")
	}

	if limit > maxSlicePageLimit {
		limit = maxSlicePageLimit
	}

	return offset, limit, true, nil
}

func monitorEntryPointValue(root any, entryPoint []string) (reflect.Value, error) {
	value := reflect.ValueOf(root)

	for _, next := range entryPoint {
		value = monitorStrip(value)

		switch value.Kind() {
		case reflect.Struct:
			value = value.FieldByName(next)
			if !value.IsValid() {
				return reflect.Value{}, fmt.Errorf("field %s not found", next)
			}
		case reflect.Map:
			key := reflect.ValueOf(next)
			keyType := value.Type().Key()
			if key.Type().AssignableTo(keyType) {
				// Use the key as-is.
			} else if key.Type().ConvertibleTo(keyType) {
				key = key.Convert(keyType)
			} else {
				return reflect.Value{}, fmt.Errorf(
					"key %s cannot be converted to %s", next, keyType)
			}

			value = value.MapIndex(key)
			if !value.IsValid() {
				return reflect.Value{}, fmt.Errorf("key %s not found", next)
			}
		case reflect.Array, reflect.Slice:
			index, err := strconv.Atoi(next)
			if err != nil {
				return reflect.Value{}, err
			}

			if index < 0 || index >= value.Len() {
				return reflect.Value{}, fmt.Errorf("index %d is not valid", index)
			}

			value = value.Index(index)
		default:
			return reflect.Value{}, fmt.Errorf("type %s is not supported", value.Type())
		}
	}

	return value, nil
}

func writeSlicePage(
	w http.ResponseWriter,
	value reflect.Value,
	offset, limit int,
) error {
	value = monitorStrip(value)
	total := value.Len()

	if offset > total {
		offset = total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	valueIDs := make([]string, 0, end-offset)
	dict := map[string]map[string]any{}

	root := map[string]any{
		"k": int(value.Kind()),
		"t": monitorTypeString(value),
		"l": total,
		"o": offset,
	}

	for index := offset; index < end; index++ {
		id := strconv.Itoa(len(valueIDs) + 1)
		valueIDs = append(valueIDs, id)
		dict[id] = monitorSethNode(value.Index(index), 1, 1)
	}

	root["v"] = valueIDs
	dict["0"] = root

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(map[string]any{
		"r":    "0",
		"dict": dict,
	})
}

func monitorSethNode(value reflect.Value, depth, maxDepth int) map[string]any {
	value = monitorStrip(value)

	if monitorIsZero(value) {
		return map[string]any{
			"k": 0,
			"t": "null",
			"v": nil,
		}
	}

	node := map[string]any{
		"k": int(value.Kind()),
		"t": monitorTypeString(value),
	}

	if monitorNeedSerializeValue(value, depth, maxDepth) {
		node["v"] = monitorValue(value)
	}

	if monitorNeedSerializeLen(value) {
		node["l"] = value.Len()
	}

	return node
}

func monitorNeedSerializeValue(
	value reflect.Value,
	depth, maxDepth int,
) bool {
	if maxDepth < 0 || depth < maxDepth {
		return true
	}

	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.Struct:
		return false
	default:
		return true
	}
}

func monitorNeedSerializeLen(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
		return true
	default:
		return false
	}
}

func monitorValue(value reflect.Value) any {
	switch value.Kind() {
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	case reflect.String:
		return value.String()
	case reflect.Chan:
		return map[string]any{
			"k": int(value.Kind()),
			"t": monitorTypeString(value),
			"l": value.Len(),
		}
	default:
		return monitorTypeString(value)
	}
}

func monitorIsZero(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}

	switch value.Kind() {
	case reflect.Chan, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func monitorStrip(value reflect.Value) reflect.Value {
	for value.IsValid() {
		switch value.Kind() {
		case reflect.Interface, reflect.Ptr:
			if value.IsNil() {
				return reflect.Value{}
			}

			value = value.Elem()
		default:
			return value
		}
	}

	return value
}

func monitorTypeString(value reflect.Value) string {
	if !value.IsValid() {
		return "null"
	}

	name := value.Type().String()
	pktPath := value.Type().PkgPath()

	if pktPath == "" {
		return name
	}

	tokens := strings.Split(pktPath, "/")
	tokens = tokens[0 : len(tokens)-1]
	pktPath = strings.Join(tokens, "/")

	return fmt.Sprintf("%s/%s", pktPath, name)
}

func (m *Monitor) hangDetectorBuffers(
	w http.ResponseWriter,
	r *http.Request,
) {
	sortMethod, limit, offset, err := buffersParseParams(r)
	if err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, "Error: %s", err)

		return
	}

	sortedBuffers := m.sortAndSelectBuffers(sortMethod, limit, offset)

	fmt.Fprintf(w, "[")

	for i, b := range sortedBuffers {
		if i > 0 {
			fmt.Fprint(w, ",")
		}

		fmt.Fprintf(w, "{\"buffer\":\"%s\",\"level\":%d,\"cap\":%d}",
			b.Name(), b.Size(), b.Capacity())
	}

	fmt.Fprint(w, "]")
}

func buffersParseParams(
	r *http.Request,
) (sortStr string, limit, offset int, err error) {
	sortMethod := r.URL.Query().Get("sort")
	if sortMethod == "" {
		sortMethod = "percent"
	}

	if sortMethod != "level" && sortMethod != "percent" {
		errStr := fmt.Sprintf(
			"Invalid sort method: %s. "+
				"Allowed values are `level` and `percent`",
			sortMethod,
		)

		return "", 0, 0, errors.New(errStr)
	}

	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "0"
	}

	limitNumber, err := strconv.Atoi(limitStr)
	if err != nil {
		return sortMethod, 0, 0, err
	}

	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = "0"
	}

	offsetNumber, err := strconv.Atoi(offsetStr)
	if err != nil {
		return sortMethod, limitNumber, 0, err
	}

	return sortMethod, limitNumber, offsetNumber, nil
}

func bufferPercent(b bufferState) float64 {
	return float64(b.Size()) / float64(b.Capacity())
}

func (m *Monitor) sortAndSelectBuffers(
	sortMethod string,
	limit, offset int,
) []bufferState {
	sortedBuffers := make([]bufferState, len(m.buffers))
	copy(sortedBuffers, m.buffers)

	if sortMethod == "level" {
		sort.Slice(sortedBuffers, func(i, j int) bool {
			sizeI := sortedBuffers[i].Size()
			sizeJ := sortedBuffers[j].Size()
			percentI := bufferPercent(sortedBuffers[i])
			percentJ := bufferPercent(sortedBuffers[j])

			if sizeI > sizeJ {
				return true
			} else if sizeI < sizeJ {
				return false
			}

			return percentI > percentJ
		})
	} else if sortMethod == "percent" {
		sort.Slice(sortedBuffers, func(i, j int) bool {
			sizeI := sortedBuffers[i].Size()
			sizeJ := sortedBuffers[j].Size()
			percentI := bufferPercent(sortedBuffers[i])
			percentJ := bufferPercent(sortedBuffers[j])

			if percentI > percentJ {
				return true
			} else if percentI < percentJ {
				return false
			}

			return sizeI > sizeJ
		})
	} else {
		panic("Invalid sort method " + sortMethod)
	}

	if offset >= len(sortedBuffers) {
		return []bufferState{}
	}

	if offset+limit > len(sortedBuffers) {
		limit = len(sortedBuffers) - offset
	}

	sortedBuffers = sortedBuffers[offset : offset+limit]

	return sortedBuffers
}

func (m *Monitor) findComponentOr404(
	w http.ResponseWriter,
	name string,
) messaging.Component {
	var component messaging.Component

	for _, c := range m.components {
		if c.Name() == name {
			component = c
		}
	}

	if component == nil {
		w.WriteHeader(http.StatusNotFound)

		_, err := w.Write([]byte("Component not found"))
		if err != nil {
			log.Panic(err)
		}
	}

	return component
}

func (m *Monitor) listProgressBars(
	w http.ResponseWriter,
	_ *http.Request,
) {
	m.progressBarsLock.Lock()
	progressBars := m.progressBars
	if progressBars == nil {
		progressBars = []*daisen2.ProgressBar{}
	}
	m.progressBarsLock.Unlock()

	b, err := json.Marshal(progressBars)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(b)
	if err != nil {
		log.Panic(err)
	}
}

type executionInfoEntry struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

func (m *Monitor) apiExecutionInfo(w http.ResponseWriter, _ *http.Request) {
	entries, err := m.readExecutionInfo()
	if err != nil {
		log.Printf("Error reading execution info: %v", err)
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(entries); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
	}
}

func (m *Monitor) readExecutionInfo() ([]executionInfoEntry, error) {
	if m.tracePath == "" {
		return []executionInfoEntry{}, nil
	}

	absPath, err := filepath.Abs(m.tracePath)
	if err != nil {
		absPath = m.tracePath
	}

	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return []executionInfoEntry{}, nil
		}

		return nil, err
	}

	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT Property, Value FROM exec_info ORDER BY rowid`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return []executionInfoEntry{}, nil
		}

		return nil, err
	}
	defer rows.Close()

	entries := []executionInfoEntry{}
	for rows.Next() {
		var entry executionInfoEntry
		if err := rows.Scan(&entry.Property, &entry.Value); err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

type resourceRsp struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemorySize uint64  `json:"memory_size"`
}

func (m *Monitor) listResources(w http.ResponseWriter, _ *http.Request) {
	pid := os.Getpid()

	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		log.Panic(err)
	}

	cpuPercent, err := proc.CPUPercent()
	if err != nil {
		log.Panic(err)
	}

	memorySize, err := proc.MemoryInfo()
	if err != nil {
		log.Panic(err)
	}

	rsp := resourceRsp{
		CPUPercent: cpuPercent,
		MemorySize: memorySize.RSS,
	}

	b, err := json.Marshal(rsp)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(b)
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) collectProfile(w http.ResponseWriter, r *http.Request) {
	seconds := 1
	if secondsStr := r.URL.Query().Get("seconds"); secondsStr != "" {
		secondsNumber, err := strconv.Atoi(secondsStr)
		if err != nil || secondsNumber < 1 {
			http.Error(w, "seconds must be a positive integer", http.StatusBadRequest)
			return
		}

		if secondsNumber > 60 {
			secondsNumber = 60
		}

		seconds = secondsNumber
	}

	buf := bytes.NewBuffer(nil)

	err := pprof.StartCPUProfile(buf)
	if err != nil {
		log.Panic(err)
	}

	time.Sleep(time.Duration(seconds) * time.Second)

	pprof.StopCPUProfile()

	prof, err := profile.ParseData(buf.Bytes())
	if err != nil {
		log.Panic(err)
	}

	b, err := json.Marshal(prof)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(b)
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) apiTraceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if m.visTracer == nil {
		fmt.Println("Error: tracer is nil")
		http.Error(w, "tracer is nil", http.StatusInternalServerError)

		return
	}

	m.visTracer.StartTracing()
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"started"}`))
}

func (m *Monitor) apiTraceEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if m.visTracer == nil {
		fmt.Println("Error: tracer is nil")
		http.Error(w, "tracer is nil", http.StatusInternalServerError)

		return
	}

	m.visTracer.StopTracing()
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"ended"}`))
}

func (m *Monitor) apiTraceIsTracing(
	w http.ResponseWriter,
	_ *http.Request,
) {
	var isTracing bool
	if m.visTracer != nil {
		isTracing = m.visTracer.IsTracing()
	}

	response := map[string]bool{"isTracing": isTracing}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
	}
}

type traceStorageRsp struct {
	Path               string `json:"path"`
	FileSizeBytes      uint64 `json:"file_size_bytes"`
	SidecarSizeBytes   uint64 `json:"sidecar_size_bytes"`
	TotalSizeBytes     uint64 `json:"total_size_bytes"`
	DiskAvailableBytes uint64 `json:"disk_available_bytes"`
	DiskTotalBytes     uint64 `json:"disk_total_bytes"`
}

func (m *Monitor) apiTraceStorage(w http.ResponseWriter, _ *http.Request) {
	dbPath := m.tracePath
	if dbPath == "" {
		dbPath = "."
	}

	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		absPath = dbPath
	}

	fileSize := pathFileSize(absPath)
	sidecarSize := pathFileSize(absPath+"-wal") + pathFileSize(absPath+"-shm")
	availableBytes, totalBytes := diskSpace(filepath.Dir(absPath))

	response := traceStorageRsp{
		Path:               absPath,
		FileSizeBytes:      fileSize,
		SidecarSizeBytes:   sidecarSize,
		TotalSizeBytes:     fileSize + sidecarSize,
		DiskAvailableBytes: availableBytes,
		DiskTotalBytes:     totalBytes,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
	}
}

func pathFileSize(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	if info.IsDir() {
		return 0
	}

	return uint64(info.Size())
}

func diskSpace(path string) (availableBytes, totalBytes uint64) {
	var stat syscall.Statfs_t

	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0
	}

	blockSize := uint64(stat.Bsize)

	return stat.Bavail * blockSize, stat.Blocks * blockSize
}

// readAll is a helper to read all bytes from an http.File.
func readAll(f http.File) ([]byte, error) {
	var buf bytes.Buffer

	_, err := buf.ReadFrom(f)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
