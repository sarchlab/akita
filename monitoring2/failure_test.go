package monitoring2

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/timing"
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

func TestInspectionDuringShutdownDoesNotFailEngine(t *testing.T) {
	for _, phase := range []string{"pause", "resume"} {
		t.Run(phase, func(t *testing.T) {
			err, _ := inspectDuringShutdown(t, phase, nil)
			if err != nil {
				t.Fatalf("healthy shutdown failed: %v", err)
			}
		})
	}
}

func TestUnexpectedInspectionPanicDuringShutdownStillFails(t *testing.T) {
	err, _ := inspectDuringShutdown(t, "resume", "broken inspected state")
	var failure *timing.FailureError
	if !errors.As(err, &failure) || failure.Cause != "broken inspected state" {
		t.Fatalf("lost original inspection failure: %v", err)
	}
}

func inspectDuringShutdown(t *testing.T, phase string, cause any) (error, int) {
	t.Helper()
	m := NewMonitor()
	e := timing.NewSerialEngine()
	m.RegisterEngine(e)
	entered, release := make(chan struct{}), make(chan struct{})
	w := httptest.NewRecorder()
	h := m.containRequests(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		if phase == "pause" {
			close(entered)
			<-release
		}
		resume := m.pauseForInspection()
		defer resume()
		if phase == "resume" {
			close(entered)
			<-release
		}
		if cause != nil {
			panic(cause)
		}
	}))
	go h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/component/test", nil))
	<-entered
	closing, stopped := make(chan struct{}), make(chan error, 1)
	go func() {
		stopped <- e.Supervisor().Close(func() {
			close(closing)
			if err := m.StopServer(); err != nil {
				panic(err)
			}
		})
	}()
	<-closing
	close(release)
	select {
	case err := <-stopped:
		return err, w.Code
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join inspection")
		return nil, 0
	}
}
