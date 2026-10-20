package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
)

// Adding hosts from the web UI: a scan may build and upload akita first.
const (
	hostScanTimeout = 5 * time.Minute
	maxRequestBytes = 64 << 10
	// requestHeader must be on requests that change something: a page of
	// another site cannot send it without this server allowing it (CORS).
	requestHeader = "X-Herd-Request"
)

// HostScanRequest asks to scan a server (POST /api/v1/hosts/scan).
type HostScanRequest struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Root     string `json:"root,omitempty"`
	AkitaDir string `json:"akita_dir,omitempty"`
}

// HostScanResponse is a scan and the host entry proposed for it.
type HostScanResponse struct {
	Scan   *herd.HostScan  `json:"scan"`
	Host   herd.HostConfig `json:"host"`
	Taken  bool            `json:"taken"`  // a host of that name is configured
	Config string          `json:"config"` // the user config it would go to
}

// HostAddRequest adds a host to the user config (POST /api/v1/hosts).
type HostAddRequest struct {
	Host herd.HostConfig `json:"host"`
}

func (m *Monitor) apiHostScan(w http.ResponseWriter, r *http.Request) {
	var req HostScanRequest
	if !readRequest(w, r, &req) {
		return
	}

	req.Address = strings.TrimSpace(req.Address)
	if err := herd.ValidateAddress(req.Address); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		req.Name = herd.HostNameFor(req.Address)
	}

	h := herd.HostConfig{Name: req.Name, Type: herd.HostSSH, Address: req.Address, Root: req.Root,
		AkitaDir: req.AkitaDir}

	b, err := backend.New(h)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), hostScanTimeout)
	defer cancel()

	scan, err := backend.Scan(ctx, b)
	if err != nil {
		writeError(w, http.StatusBadGateway, errorText(err))
		return
	}

	root := req.Root
	if root == "" {
		root = herd.PickRoot(scan)
	}

	entry := herd.SuggestHost(req.Name, req.Address, root, scan)
	entry.AkitaDir = req.AkitaDir

	writeJSON(w, http.StatusOK, HostScanResponse{
		Scan: scan, Host: entry, Taken: m.configured(req.Name), Config: herd.UserConfigPath(),
	})
}

func (m *Monitor) apiHostAdd(w http.ResponseWriter, r *http.Request) {
	var req HostAddRequest
	if !readRequest(w, r, &req) {
		return
	}

	h := req.Host

	switch {
	case h.Type != herd.HostSSH:
		writeError(w, http.StatusBadRequest, "only ssh hosts can be added here")
		return
	case herd.HostNameFor(h.Name) != h.Name || h.Name == "":
		writeError(w, http.StatusBadRequest, "a host name has letters, digits, - and _ only")
		return
	case m.configured(h.Name):
		writeError(w, http.StatusConflict, "a host named "+h.Name+" is configured already")
		return
	}

	if err := herd.ValidateAddress(h.Address); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	path := herd.UserConfigPath()
	if err := herd.AddHosts(path, []herd.HostConfig{h}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := m.AddHost(h); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"added": h.Name, "config": path})
}

// configured tells whether the user config has a host of that name, or
// the monitor polls one.
func (m *Monitor) configured(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.cfg.Host(name); ok {
		return true
	}

	for _, h := range m.hosts {
		if h.backend.Host().Name == name {
			return true
		}
	}

	return false
}

// readRequest decodes the JSON body of a request that changes something,
// after checking that it comes from this UI: a POST with a JSON body and
// requestHeader, and, when the browser names its origin, this server.
func readRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return false
	}

	if r.Header.Get(requestHeader) == "" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, http.StatusForbidden, "requests that change something come from the Herd UI only")
		return false
	}

	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || u.Host != r.Host {
			writeError(w, http.StatusForbidden, "requests from other sites are refused")
			return false
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err == nil {
		err = json.Unmarshal(body, v)
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, "bad request: "+err.Error())
		return false
	}

	return true
}
