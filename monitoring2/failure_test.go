package monitoring2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/timing"
)

func TestExecutionInfoUsesLiteralRecordingPath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "trace.sqlite3")
	name := base + "?literal#"
	r, err := datarecording.NewDataRecorder(name)
	if err != nil {
		t.Fatal(err)
	}
	entry := executionInfoEntry{Property: "Command", Value: "expected recording"}
	if err := r.CreateTable("exec_info", entry); err != nil {
		t.Fatal(err)
	}
	if err := r.InsertData("exec_info", entry); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m := NewMonitor()
	m.SetTraceDBPath(name + ".sqlite3")
	w := httptest.NewRecorder()
	m.apiExecutionInfo(w, httptest.NewRequest(http.MethodGet, "/api/execution/info", nil))
	var entries []executionInfoEntry
	err = json.Unmarshal(w.Body.Bytes(), &entries)
	if err != nil || w.Code != http.StatusOK || len(entries) != 1 || entries[0] != entry {
		t.Fatalf("read wrong recording: status=%d body=%s err=%v", w.Code, w.Body.String(), err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("monitor created an unintended file: %v", err)
	}
}

func TestBufferPaginationCannotFailSimulation(t *testing.T) {
	for _, tc := range []struct {
		query  string
		status int
		count  int
	}{
		{"limit=-1", http.StatusBadRequest, 0},
		{"offset=-1&limit=1", http.StatusBadRequest, 0},
		{fmt.Sprintf("offset=1&limit=%d", int(^uint(0)>>1)), http.StatusOK, 1},
	} {
		t.Run(tc.query, func(t *testing.T) {
			m := NewMonitor()
			e := timing.NewSerialEngine()
			m.RegisterEngine(e)
			m.RegisterComponent(newBufferOnlyComponent("a", 10, 5))
			m.RegisterComponent(newBufferOnlyComponent("b", 10, 7))
			h := m.containRequests(http.HandlerFunc(m.hangDetectorBuffers))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/hangdetector/buffers?"+tc.query, nil))
			if w.Code != tc.status || e.Supervisor().Err() != nil {
				t.Fatalf("status=%d failure=%v body=%s", w.Code, e.Supervisor().Err(), w.Body.String())
			}
			if tc.status == http.StatusOK {
				var buffers []bufferRsp
				if err := json.Unmarshal(w.Body.Bytes(), &buffers); err != nil || len(buffers) != tc.count {
					t.Fatalf("invalid page: %s, %v", w.Body.String(), err)
				}
			}
			if err := e.Run(); err != nil {
				t.Fatalf("request invalidated engine: %v", err)
			}
		})
	}
}

func TestProfileCaptureHonorsCancellation(t *testing.T) {
	m := NewMonitor()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/profile?seconds=2", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	started := time.Now()
	m.collectProfile(w, r)
	if w.Code != http.StatusRequestTimeout || time.Since(started) > time.Second {
		t.Fatalf("profile ignored cancellation: status=%d elapsed=%s", w.Code, time.Since(started))
	}
}

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
