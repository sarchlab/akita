package monitoring2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/timing"
)

type inspectionHandler func(timing.Event)

func (h inspectionHandler) Handle(e timing.Event) { h(e) }

type slowInspectionWriter struct {
	*httptest.ResponseRecorder
	writing chan struct{}
	release chan struct{}
}

func (w *slowInspectionWriter) Write(data []byte) (int, error) {
	close(w.writing)
	<-w.release
	return w.ResponseRecorder.Write(data)
}

func waitInspection[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("monitor inspection did not complete")
		var zero T
		return zero
	}
}

func TestMonitorSnapshotDoesNotWaitForSlowClient(t *testing.T) {
	for name, e := range map[string]interface {
		timing.Engine
		timing.HandlerRegistrar
	}{"serial": timing.NewSerialEngine(), "parallel": timing.NewParallelEngine()} {
		t.Run(name, func(t *testing.T) {
			m := NewMonitor()
			m.RegisterEngine(e)
			component := newSliceFieldComponent("snapshot", []int{10, 20})
			m.RegisterComponent(component)
			e.RegisterHandler("model", inspectionHandler(func(timing.Event) {
				component.State.Values[0] = 30
				component.State.Values[1] = 40
			}))
			e.Schedule(timing.EventBase{Time_: 1, HandlerID_: "model"})
			if err := e.Pause(); err != nil {
				t.Fatal(err)
			}
			runDone := make(chan error, 1)
			go func() { runDone <- e.Run() }()
			writer := &slowInspectionWriter{
				ResponseRecorder: httptest.NewRecorder(),
				writing:          make(chan struct{}), release: make(chan struct{}),
			}
			requestDone := make(chan struct{})
			go func() {
				m.listFieldValue(writer, httptest.NewRequest(http.MethodGet,
					`/api/field/{"comp_name":"snapshot","field_name":"State.Values"}`, nil))
				close(requestDone)
			}()
			waitInspection(t, writer.writing)
			if !e.IsPaused() {
				t.Fatal("inspection resumed the user's paused engine")
			}
			if err := e.Continue(); err != nil {
				t.Fatal(err)
			}
			if err := waitInspection(t, runDone); err != nil {
				t.Fatal(err)
			}
			// The run has changed the live slice while the HTTP writer is blocked.
			close(writer.release)
			waitInspection(t, requestDone)
			if writer.Code != http.StatusOK {
				t.Fatalf("inspection failed: %s", writer.Body)
			}
			var response fieldValueResponse
			if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			root := response.Dict[response.R]
			var ids []string
			if err := json.Unmarshal(root.V, &ids); err != nil {
				t.Fatal(err)
			}
			assertSlicePageValues(t, response, ids, []int{10, 20})
			t.Log("HTTP snapshot retained [10,20]; engine completed and changed live state to [30,40] " +
				"before the slow client received its response")
		})
	}
}

func TestMonitorReportsHandlerRequestedPause(t *testing.T) {
	e := timing.NewSerialEngine()
	if err := e.Pause(); err != nil {
		t.Fatal(err)
	}
	m := NewMonitor()
	m.RegisterEngine(e)
	w := httptest.NewRecorder()
	m.apiEngineState(w, httptest.NewRequest(http.MethodGet, "/api/engine/state", nil))
	var state engineStateRsp
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Paused || state.State != "paused" {
		t.Fatal("monitor reported its own stale pause state")
	}
}
