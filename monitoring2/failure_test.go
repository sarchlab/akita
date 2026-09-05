package monitoring2

import (
	"github.com/sarchlab/akita/v5/timing"
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

func TestMonitorPanicFailsStandaloneManagedEngine(t *testing.T) {
	m := NewMonitor()
	e := timing.NewSerialEngine()
	m.RegisterEngine(e)
	h := m.containRequests(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("invalid monitored model")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tick/test", nil))
	if w.Code != http.StatusInternalServerError || e.Run() == nil {
		t.Fatal("monitor hid the standalone engine failure")
	}
}
