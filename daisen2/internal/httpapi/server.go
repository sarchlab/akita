// Package httpapi provides a trace visualization server for Akita simulations.
// It serves replay data from a SQLite file.
package httpapi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sarchlab/akita/v5/daisen2/static"
	"github.com/sarchlab/akita/v5/sourcefs"
)

// Server is the Daisen replay server. It reads trace data from a SQLite file
// and serves it over HTTP for the Daisen dashboard.
type Server struct {
	addr        string
	traceReader *SQLiteTraceReader
	fs          http.FileSystem
	httpServer  *http.Server

	// codeSource is the simulator source recorded in the loaded trace, read once
	// on startup. The code-reading tools (code_search / code_read) search it.
	// Non-nil; IsEmpty reports when the trace carries no source.
	codeSource *sourcefs.Source

	// captures correlates a pending agent capture request (keyed by id) with the
	// browser that will POST the image back to /api/agent/capture (Phase 5).
	captures   map[string]chan string
	capturesMu sync.Mutex
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
		addr:        addr,
		traceReader: reader,
		fs:          static.GetAssets(),
		codeSource:  loadCodeSource(reader),
	}
}

// loadCodeSource reads the source recorded in the trace (if any) and logs what
// is available. It never fails the server: an unreadable or missing source
// table yields an empty Source, and DaisenBot reports the gap rather than
// erroring.
func loadCodeSource(reader *SQLiteTraceReader) *sourcefs.Source {
	src, err := sourcefs.OpenTraceSource(reader.DB)
	if err != nil {
		log.Printf("DaisenBot: could not read recorded source from trace: %v", err)
		return &sourcefs.Source{}
	}
	if src.IsEmpty() {
		log.Printf("DaisenBot: no source recorded in this trace")
	} else {
		log.Printf("DaisenBot: recorded source available — %d files across %v",
			src.Files, src.Roots)
	}
	return src
}

// Start starts the HTTP server in replay mode. It listens on the configured
// address and blocks until the server is shut down.
func (s *Server) Start() {
	mux := s.setupRoutes()
	s.startReplayServer(mux)
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

func (s *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Trace endpoints
	mux.HandleFunc("/api/trace", s.httpTrace)
	mux.HandleFunc("/api/trace_range", s.httpTraceTimeRange)
	mux.HandleFunc("/api/trace_info", s.httpTraceInfo)
	mux.HandleFunc("/api/compnames", s.httpComponentNames)
	mux.HandleFunc("/api/compinfo", s.httpComponentInfo)
	mux.HandleFunc("/api/component_timeline", s.httpComponentTimeline)
	mux.HandleFunc("/api/top_blocking_resources", s.httpTopBlockingResources)
	mux.HandleFunc("/api/resource_blocking", s.httpResourceBlocking)
	mux.HandleFunc("/api/resource_tasks", s.httpResourceTasks)
	mux.HandleFunc("/api/segments", s.httpSegments)
	mux.HandleFunc("/api/sim_info", s.httpSimInfo)
	mux.HandleFunc("/api/topology", s.httpTopology)
	mux.HandleFunc("/api/components", s.httpComponents)
	mux.HandleFunc("/api/db_info", s.httpDBInfo)
	mux.HandleFunc("/api/db_activity", s.httpDBActivity)
	mux.HandleFunc("/api/code/ls", s.httpCodeLs)
	mux.HandleFunc("/api/code/read", s.httpCodeRead)

	// Chat / LLM proxy endpoints. The LLM provider is configured entirely from
	// the frontend; the server holds no credentials.
	mux.HandleFunc("/api/gpt", s.httpChatProxy)
	mux.HandleFunc("/api/models", s.httpListModels)
	// The browser POSTs agent view captures (screenshots / off-screen renders) here.
	mux.HandleFunc("/api/agent/capture", s.httpAgentCapture)

	// Static assets / SPA fallback
	fServer := http.FileServer(s.fs)
	mux.HandleFunc("/dashboard", s.serveIndex)
	mux.HandleFunc("/component", s.serveIndex)
	mux.HandleFunc("/task", s.serveIndex)
	mux.HandleFunc("/resource", s.serveIndex)
	// Enlarged single-widget pages (/view/<widget>). A trailing-slash pattern
	// matches the whole subtree so a hard refresh serves the SPA shell.
	mux.HandleFunc("/view/", s.serveIndex)
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

// httpTraceInfo returns a stable identifier for the loaded trace, used by the
// frontend to scope browser-stored DaisenBot conversations to this trace. It is
// read-only: the id is derived from the trace file name and the trace contents
// are never touched.
func (s *Server) httpTraceInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := ""
	if s.traceReader != nil {
		path := s.traceReader.filename
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		// Suffix a short hash of the absolute path so two different traces that share
		// a basename (e.g. repeated experiment outputs named trace.sqlite in separate
		// directories) get distinct ids and don't share browser-stored conversations.
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		sum := sha256.Sum256([]byte(abs))
		id = fmt.Sprintf("%s-%x", base, sum[:4])
	}
	fmt.Fprintf(w, `{"traceId":%q}`, id)
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	f, err := s.fs.Open("index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	p, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "failed to read index", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(p)
}

func dieOnErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
