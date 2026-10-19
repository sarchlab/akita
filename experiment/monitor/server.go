package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/sarchlab/akita/v5/experiment/static"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// Server timings.
const (
	shutdownTimeout   = 5 * time.Second
	readHeaderTimeout = 10 * time.Second
)

// handler is the HTTP handler of the web UI (its files from assets) and
// /api/v1.
func (m *Monitor) handler(assets http.FileSystem) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", m.apiStatus)
	mux.HandleFunc("/api/v1/run", m.apiRun)
	mux.HandleFunc("/api/v1/campaign", m.apiCampaign)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "no such API route")
	})
	mux.Handle("/", spaHandler(assets))

	return mux
}

// ServeListener polls the hosts and serves the UI on a listener until ctx
// is done.
func (m *Monitor) ServeListener(ctx context.Context, ln net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go m.Run(ctx)

	srv := &http.Server{Handler: m.handler(static.Assets()), ReadHeaderTimeout: readHeaderTimeout}

	go func() {
		<-ctx.Done()

		sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer scancel()

		_ = srv.Shutdown(sctx)
	}()

	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (m *Monitor) apiStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, m.Status())
}

// runQuery reads and checks host, project, and name.
func runQuery(w http.ResponseWriter, r *http.Request) (host, project, name string, refresh, ok bool) {
	q := r.URL.Query()
	host, project, name = q.Get("host"), q.Get("project"), q.Get("name")

	for _, s := range []string{host, project, name} {
		if workspace.ValidateRunName(s) != nil {
			writeError(w, http.StatusBadRequest, "need host, project, and name")
			return "", "", "", false, false
		}
	}

	return host, project, name, q.Get("refresh") == "1", true
}

func (m *Monitor) apiRun(w http.ResponseWriter, r *http.Request) {
	host, project, name, refresh, ok := runQuery(w, r)
	if !ok {
		return
	}

	d, err := m.RunDetail(r.Context(), host, project, name, refresh)
	if err != nil {
		writeFetchError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, d)
}

func (m *Monitor) apiCampaign(w http.ResponseWriter, r *http.Request) {
	host, project, name, refresh, ok := runQuery(w, r)
	if !ok {
		return
	}

	c, err := m.Campaign(r.Context(), host, project, name, refresh)
	if err != nil {
		writeFetchError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, c)
}

func writeFetchError(w http.ResponseWriter, err error) {
	var uh *unknownHostError
	if errors.As(err, &uh) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeError(w, http.StatusBadGateway, errorText(err))
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		code = http.StatusInternalServerError
		body = []byte(`{"error":"cannot encode the response"}`)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

// spaHandler serves the UI's files; unknown paths get index.html (client
// routes of the single-page app).
func spaHandler(assets http.FileSystem) http.Handler {
	files := http.FileServer(assets)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if p != "/" && !assetExists(assets, p) {
			serveIndex(w, r, assets)
			return
		}

		if p == "/" || p == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}

		files.ServeHTTP(w, r)
	})
}

func assetExists(assets http.FileSystem, p string) bool {
	f, err := assets.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()

	st, err := f.Stat()

	return err == nil && !st.IsDir()
}

func serveIndex(w http.ResponseWriter, r *http.Request, assets http.FileSystem) {
	f, err := assets.Open("/index.html")
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			code = http.StatusNotFound
		}

		http.Error(w, "UI not available", code)

		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		http.Error(w, "UI not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", st.ModTime(), f)
}
