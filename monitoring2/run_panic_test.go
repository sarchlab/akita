package monitoring2

import (
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/timing"
)

type runPanicHandler struct{}

func (runPanicHandler) Handle(timing.Event) { panic("invalid monitored model") }

type runLog chan string

func (c runLog) Write(p []byte) (int, error) { c <- string(p); return len(p), nil }

func TestMonitorReportsRunFailureWithoutRepanicking(t *testing.T) {
	e := timing.NewSerialEngine()
	e.RegisterHandler("model", runPanicHandler{})
	e.Schedule(timing.EventBase{Time_: 1, HandlerID_: "model"})
	m := NewMonitor()
	m.RegisterEngine(e)
	messages := make(runLog, 1)
	previous := log.Writer()
	log.SetOutput(messages)
	defer log.SetOutput(previous)
	m.run(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/run", nil))
	select {
	case message := <-messages:
		if !strings.Contains(message, "simulation run failed:") || !strings.Contains(message, "invalid monitored model") {
			t.Fatalf("missing run diagnostic: %s", message)
		}
		t.Log("monitor reported the contained engine panic without crashing the process")
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not report the failed run")
	}
}
