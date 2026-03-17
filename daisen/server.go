// Package daisen provides a trace visualization server for Akita simulations.
// It supports two modes: "live" mode with monitoring during simulation, and
// "replay" mode for viewing trace data from a SQLite file.
package daisen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	// Enable profiling
	_ "net/http/pprof"

	"github.com/google/pprof/profile"
	"github.com/sarchlab/akita/v5/daisen/static"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/shirou/gopsutil/process"
	"github.com/syifan/goseth"
)

// Server is the main Daisen server. It can operate in "live" mode (attached
// to a running simulation engine with monitoring) or "replay" mode (standalone
// trace viewer reading from a SQLite file).
type Server struct {
	mode        string // "live" or "replay"
	addr        string
	traceReader *SQLiteTraceReader
	fs          http.FileSystem
	httpServer  *http.Server

	// Live mode fields (from monitoring.Monitor)
	engine     sim.Engine
	components []sim.Component
	buffers    []queueing.BufferState
	tracer     *tracing.DBTracer

	progressBarsLock sync.Mutex
	progressBars     []*ProgressBar

	portNumber  int
	userSetPort bool
}

// NewReplayServer creates a new Server in replay mode. It reads trace data
// from the given SQLite file and serves it on the given address.
func NewReplayServer(sqliteFile, addr string) *Server {
	if sqliteFile == "" {
		panic("must specify a SQLite file")
	}

	reader := NewSQLiteTraceReader(sqliteFile)
	reader.Init()

	return &Server{
		mode:        "replay",
		addr:        addr,
		traceReader: reader,
		fs:          static.GetAssets(),
	}
}

// NewLiveServer creates a new Server in live mode. It monitors the given
// simulation engine and serves monitoring + trace endpoints.
func NewLiveServer(engine sim.Engine, addr string) *Server {
	return &Server{
		mode:       "live",
		addr:       addr,
		engine:     engine,
		fs:         static.GetAssets(),
		portNumber: 32776,
	}
}

// WithPortNumber sets the port number for the live mode server.
func (s *Server) WithPortNumber(portNumber int) *Server {
	if portNumber < 1000 {
		fmt.Fprintf(os.Stderr,
			"Port number %d is assigned to the server, "+
				"which is not allowed. Using a random port instead.\n",
			portNumber,
		)
		portNumber = 0
	}
	s.portNumber = portNumber
	s.userSetPort = true
	return s
}

// RegisterEngine registers the engine that is used in the simulation.
func (s *Server) RegisterEngine(e sim.Engine) {
	s.engine = e
}

// RegisterComponent registers a component to be monitored (live mode only).
func (s *Server) RegisterComponent(c sim.Component) {
	s.components = append(s.components, c)
	s.registerBuffers(c)
}

func (s *Server) registerBuffers(c sim.Component) {
	s.registerComponentOrPortBuffers(c)
	for _, p := range c.Ports() {
		s.registerComponentOrPortBuffers(p)
	}
}

func (s *Server) registerComponentOrPortBuffers(c any) {
	v := reflect.ValueOf(c).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := field.Type()
		bufferType := reflect.TypeOf((*queueing.BufferState)(nil)).Elem()
		if fieldType == bufferType {
			fieldRef := reflect.NewAt(
				field.Type(),
				unsafe.Pointer(field.UnsafeAddr()),
			).Elem().Interface().(queueing.BufferState)
			s.buffers = append(s.buffers, fieldRef)
		}
	}
}

// RegisterVisTracer registers a tracer instance to the server (live mode only).
func (s *Server) RegisterVisTracer(tr *tracing.DBTracer) {
	s.tracer = tr
}

// SetTraceDBPath opens a read-only SQLite connection to the trace database.
// In live mode, this allows the server to serve trace endpoints while the
// DBTracer is actively writing data. Uses WAL mode for concurrent access.
func (s *Server) SetTraceDBPath(dbPath string) {
	reader := NewSQLiteTraceReader(dbPath)
	reader.InitReadOnly()
	s.traceReader = reader
}

// CreateProgressBar creates a new progress bar (live mode only).
func (s *Server) CreateProgressBar(name string, total uint64) *ProgressBar {
	bar := &ProgressBar{
		ID:    sim.GetIDGenerator().Generate(),
		Name:  name,
		Total: total,
	}

	s.progressBarsLock.Lock()
	defer s.progressBarsLock.Unlock()

	s.progressBars = append(s.progressBars, bar)
	return bar
}

// CompleteProgressBar removes a bar from the progress list.
func (s *Server) CompleteProgressBar(pb *ProgressBar) {
	s.progressBarsLock.Lock()
	defer s.progressBarsLock.Unlock()

	newBars := make([]*ProgressBar, 0, len(s.progressBars)-1)
	for _, b := range s.progressBars {
		if b != pb {
			newBars = append(newBars, b)
		}
	}
	s.progressBars = newBars
}

// Start starts the HTTP server. In replay mode it listens on the configured
// address. In live mode it finds an available port.
func (s *Server) Start() {
	mux := s.setupRoutes()

	if s.mode == "replay" {
		s.startReplayServer(mux)
	} else {
		s.startLiveServer(mux)
	}
}

// StartServer starts the server (alias for Start, for backward compatibility
// with monitoring.Monitor).
func (s *Server) StartServer() {
	s.Start()
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := s.httpServer.Shutdown(ctx)
		if err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}
}

// StopServer stops the server (alias for Stop, for backward compatibility
// with monitoring.Monitor).
func (s *Server) StopServer() {
	s.Stop()
}

func (s *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Shared endpoints (both modes)
	mux.HandleFunc("/api/mode", s.apiMode)

	// Trace endpoints (both modes, but traceReader must be set)
	mux.HandleFunc("/api/trace", s.httpTrace)
	mux.HandleFunc("/api/compnames", s.httpComponentNames)
	mux.HandleFunc("/api/compinfo", s.httpComponentInfo)
	mux.HandleFunc("/api/segments", s.httpSegments)

	// GPT proxy endpoints (both modes)
	mux.HandleFunc("/api/gpt", s.httpGPTProxy)
	mux.HandleFunc("/api/githubisavailable", s.httpGithubIsAvailableProxy)
	mux.HandleFunc("/api/checkenv", s.httpCheckEnvFile)

	// Monitoring/live-mode endpoints
	monitoringPaths := []string{
		"/api/pause",
		"/api/continue",
		"/api/now",
		"/api/run",
		"/api/tick/",
		"/api/list_components",
		"/api/component/",
		"/api/field/",
		"/api/hangdetector/buffers",
		"/api/progress",
		"/api/resource",
		"/api/profile",
		"/api/trace/start",
		"/api/trace/end",
		"/api/trace/is_tracing",
	}

	if s.mode == "live" {
		mux.HandleFunc("/api/pause", s.pauseEngine)
		mux.HandleFunc("/api/continue", s.continueEngine)
		mux.HandleFunc("/api/now", s.now)
		mux.HandleFunc("/api/run", s.run)
		mux.HandleFunc("/api/tick/", s.tick)
		mux.HandleFunc("/api/list_components", s.listComponents)
		mux.HandleFunc("/api/component/", s.listComponentDetails)
		mux.HandleFunc("/api/field/", s.listFieldValue)
		mux.HandleFunc("/api/hangdetector/buffers", s.hangDetectorBuffers)
		mux.HandleFunc("/api/progress", s.listProgressBars)
		mux.HandleFunc("/api/resource", s.listResources)
		mux.HandleFunc("/api/profile", s.collectProfile)
		mux.HandleFunc("/api/trace/start", s.apiTraceStart)
		mux.HandleFunc("/api/trace/end", s.apiTraceEnd)
		mux.HandleFunc("/api/trace/is_tracing", s.apiTraceIsTracing)
	} else {
		// In replay mode, register monitoring endpoints with 503 response
		for _, path := range monitoringPaths {
			mux.HandleFunc(path, liveOnlyHandler)
		}
	}

	// Static assets / SPA fallback
	fServer := http.FileServer(s.fs)
	mux.HandleFunc("/dashboard", s.serveIndex)
	mux.HandleFunc("/component", s.serveIndex)
	mux.HandleFunc("/task", s.serveIndex)
	mux.Handle("/", fServer)

	return mux
}

func (s *Server) startReplayServer(mux *http.ServeMux) {
	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	fmt.Printf("Listening %s\n", s.addr)

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		dieOnErr(err)
	}
}

func (s *Server) startLiveServer(mux *http.ServeMux) {
	var listener net.Listener
	var err error

	if s.userSetPort {
		actualPort := ":" + strconv.Itoa(s.portNumber)
		listener, err = net.Listen("tcp", actualPort)
		dieOnErr(err)
	} else {
		startPort := 32776
		maxAttempts := 100
		for i := 0; i < maxAttempts; i++ {
			tryPort := startPort + i
			listener, err = net.Listen("tcp", ":"+strconv.Itoa(tryPort))
			if err == nil {
				break
			}
		}
		if err != nil {
			dieOnErr(fmt.Errorf(
				"failed to find available port after %d attempts: %w",
				maxAttempts, err))
		}
	}

	fmt.Fprintf(
		os.Stderr,
		"Monitoring simulation with http://localhost:%d\n",
		listener.Addr().(*net.TCPAddr).Port)

	s.httpServer = &http.Server{Handler: mux}

	go func() {
		err = s.httpServer.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			dieOnErr(err)
		}
	}()
}

// ---- Shared handlers ----

func (s *Server) apiMode(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"mode":%q}`, s.mode)
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	f, err := s.fs.Open("index.html")
	dieOnErr(err)

	p, err := io.ReadAll(f)
	dieOnErr(err)

	_, err = w.Write(p)
	dieOnErr(err)
}

// ---- Live mode handlers (ported from monitoring.Monitor) ----

func (s *Server) pauseEngine(w http.ResponseWriter, _ *http.Request) {
	s.engine.Pause()
	_, err := w.Write(nil)
	dieOnErr(err)
}

func (s *Server) continueEngine(w http.ResponseWriter, _ *http.Request) {
	s.engine.Continue()
	_, err := w.Write(nil)
	dieOnErr(err)
}

func (s *Server) now(w http.ResponseWriter, _ *http.Request) {
	nowTime := s.engine.CurrentTime()
	fmt.Fprintf(w, "{\"now\":%d}", nowTime)
}

func (s *Server) run(_ http.ResponseWriter, _ *http.Request) {
	go func() {
		err := s.engine.Run()
		if err != nil {
			panic(err)
		}
	}()
}

func (s *Server) listComponents(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprint(w, "[")
	for i, c := range s.components {
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

func (s *Server) tick(w http.ResponseWriter, r *http.Request) {
	compName := strings.TrimPrefix(r.URL.Path, "/api/tick/")

	comp := s.findComponentOr404(w, compName)
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

func (s *Server) listComponentDetails(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/component/")

	component := s.findComponentOr404(w, name)
	if component == nil {
		return
	}

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)
	err := serializer.Serialize(w)
	dieOnErr(err)
}

type fieldReq struct {
	CompName  string `json:"comp_name,omitempty"`
	FieldName string `json:"field_name,omitempty"`
}

func (s *Server) listFieldValue(w http.ResponseWriter, r *http.Request) {
	jsonString := strings.TrimPrefix(r.URL.Path, "/api/field/")
	req := fieldReq{}

	err := json.Unmarshal([]byte(jsonString), &req)
	if err != nil {
		dieOnErr(err)
	}

	name := req.CompName
	fields := strings.Split(req.FieldName, ".")

	component := s.findComponentOr404(w, name)
	if component == nil {
		return
	}

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)

	err = serializer.SetEntryPoint(fields)
	dieOnErr(err)

	err = serializer.Serialize(w)
	dieOnErr(err)
}

func (s *Server) hangDetectorBuffers(w http.ResponseWriter, r *http.Request) {
	sortMethod, limit, offset, err := s.buffersParseParams(r, w)
	if err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, "Error: %s", err)
		return
	}

	sortedBuffers := s.sortAndSelectBuffers(sortMethod, limit, offset)

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

func (*Server) buffersParseParams(
	r *http.Request,
	_ http.ResponseWriter,
) (sortStr string, limit, offset int, err error) {
	sortMethod := r.URL.Query().Get("sort")
	if sortMethod == "" {
		sortMethod = "percent"
	}

	if sortMethod != "level" && sortMethod != "percent" {
		errStr := fmt.Sprintf(
			"Invalid sort method: %s. Allowed values are `level` and `percent`",
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

func bufferPercent(b queueing.BufferState) float64 {
	return float64(b.Size()) / float64(b.Capacity())
}

func (s *Server) sortAndSelectBuffers(
	sortMethod string,
	limit, offset int,
) []queueing.BufferState {
	sortedBuffers := make([]queueing.BufferState, len(s.buffers))
	copy(sortedBuffers, s.buffers)

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
			} else {
				return percentI > percentJ
			}
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
			} else {
				return sizeI > sizeJ
			}
		})
	} else {
		panic("Invalid sort method " + sortMethod)
	}

	if offset >= len(sortedBuffers) {
		return []queueing.BufferState{}
	}

	if offset+limit > len(sortedBuffers) {
		limit = len(sortedBuffers) - offset
	}

	sortedBuffers = sortedBuffers[offset : offset+limit]
	return sortedBuffers
}

func (s *Server) findComponentOr404(
	w http.ResponseWriter,
	name string,
) sim.Component {
	var component sim.Component

	for _, c := range s.components {
		if c.Name() == name {
			component = c
		}
	}

	if component == nil {
		w.WriteHeader(http.StatusNotFound)
		_, err := w.Write([]byte("Component not found"))
		dieOnErr(err)
	}

	return component
}

func (s *Server) listProgressBars(w http.ResponseWriter, _ *http.Request) {
	b, err := json.Marshal(s.progressBars)
	dieOnErr(err)

	_, err = w.Write(b)
	dieOnErr(err)
}

type resourceRsp struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemorySize uint64  `json:"memory_size"`
}

func (s *Server) listResources(w http.ResponseWriter, _ *http.Request) {
	pid := os.Getpid()
	proc, err := process.NewProcess(int32(pid))
	dieOnErr(err)

	cpuPercent, err := proc.CPUPercent()
	dieOnErr(err)

	memorySize, err := proc.MemoryInfo()
	dieOnErr(err)

	rsp := resourceRsp{
		CPUPercent: cpuPercent,
		MemorySize: memorySize.RSS,
	}

	b, err := json.Marshal(rsp)
	dieOnErr(err)

	_, err = w.Write(b)
	dieOnErr(err)
}

func (s *Server) collectProfile(w http.ResponseWriter, _ *http.Request) {
	buf := bytes.NewBuffer(nil)

	err := pprof.StartCPUProfile(buf)
	dieOnErr(err)

	time.Sleep(time.Second)

	pprof.StopCPUProfile()

	prof, err := profile.ParseData(buf.Bytes())
	dieOnErr(err)

	b, err := json.Marshal(prof)
	dieOnErr(err)

	_, err = w.Write(b)
	dieOnErr(err)
}

func (s *Server) apiTraceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.tracer == nil {
		fmt.Println("Error: tracer is nil")
		http.Error(w, "tracer is nil", http.StatusInternalServerError)
		return
	}

	s.tracer.StartTracing()
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"started"}`))
}

func (s *Server) apiTraceEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.tracer == nil {
		fmt.Println("Error: tracer is nil")
		http.Error(w, "tracer is nil", http.StatusInternalServerError)
		return
	}

	s.tracer.StopTracing()
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"ended"}`))
}

func (s *Server) apiTraceIsTracing(w http.ResponseWriter, _ *http.Request) {
	var isTracing bool
	if s.tracer != nil {
		isTracing = s.tracer.IsTracing()
	}

	response := map[string]bool{"isTracing": isTracing}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// liveOnlyHandler returns 503 Service Unavailable for monitoring endpoints
// that are only available in live mode.
func liveOnlyHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint only available in live mode",
		http.StatusServiceUnavailable)
}

func dieOnErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
