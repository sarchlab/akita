package monitoring2

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStopServerJoinsAdmittedRequests(t *testing.T) {
	m := NewMonitor()
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := m.containRequests(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	go func() {
		defer close(exited)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- m.StopServer() }()
	select {
	case err := <-stopped:
		close(release)
		t.Fatalf("returned before request settled: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not settle")
	}
	<-exited
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("accepted request after stop")
	}
}
