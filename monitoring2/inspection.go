package monitoring2

import (
	"bytes"
	"net/http"
)

// inspectionResponse is private to the boundary callback. The network writer
// is touched only after inspection finishes, so slow clients cannot stop Run.
type inspectionResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *inspectionResponse) Header() http.Header { return r.header }

func (r *inspectionResponse) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *inspectionResponse) Write(p []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return r.body.Write(p)
}

func (m *Monitor) inspectResponse(
	w http.ResponseWriter,
	r *http.Request,
	serialize func(http.ResponseWriter),
) {
	snapshot := &inspectionResponse{header: make(http.Header)}
	err := m.engine.Inspect(r.Context(), func() error {
		serialize(snapshot)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for name, values := range snapshot.header {
		w.Header()[name] = values
	}
	if snapshot.status != 0 {
		w.WriteHeader(snapshot.status)
	}
	_, _ = w.Write(snapshot.body.Bytes())
}
