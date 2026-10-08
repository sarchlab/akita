package sim_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type lifecycleMonitor struct {
	events    []string
	runtime   *sim.Simulation
	path      string
	component naming.Named
	port      sim.Port
}

func (m *lifecycleMonitor) Start(s *sim.Simulation) {
	if s.Engine() == nil || s.VisTracer() == nil || s.DataRecorder() == nil {
		panic("monitor started before runtime services were ready")
	}
	m.runtime, m.path = s, s.TraceDBPath()
	m.events = append(m.events, "start")
}

func (m *lifecycleMonitor) RegisterComponent(c naming.Named) {
	m.events = append(m.events, "component")
	m.component = c
}

func (m *lifecycleMonitor) RegisterPort(p sim.Port) {
	m.events = append(m.events, "port")
	m.port = p
}

func (m *lifecycleMonitor) Stop() {
	m.events = append(m.events, "stop")
	// Monitors stop while the recording is still usable.
	recorder := m.runtime.DataRecorder()
	recorder.CreateTable("monitor_shutdown", struct{ Stopped bool }{})
	recorder.InsertData("monitor_shutdown", struct{ Stopped bool }{true})
	recorder.Flush()
}

type namedComponent string

func (c namedComponent) Name() string { return string(c) }

func TestInjectedMonitorLifecycle(t *testing.T) {
	output := filepath.Join(t.TempDir(), "recording")
	monitor := &lifecycleMonitor{}
	s := sim.MakeBuilder().WithOutputFileName(output).WithMonitor(monitor).Build()
	if s.Monitor() != monitor || monitor.runtime != s || monitor.path != output+".sqlite3" {
		t.Fatal("monitor did not receive the simulation and recording destination")
	}
	component := namedComponent("Agent")
	port := twowaybuffered.NewPort("Agent.Top", 2, 2)
	s.RegisterComponent(component)
	s.RegisterPort(port)
	s.Terminate()
	if monitor.component != component || monitor.port != port {
		t.Fatal("monitor did not receive registered component and port")
	}
	want := []string{"start", "component", "port", "stop"}
	if !reflect.DeepEqual(monitor.events, want) {
		t.Fatalf("lifecycle = %v, want %v", monitor.events, want)
	}
}

func TestMonitoringIsOptIn(t *testing.T) {
	s := sim.MakeBuilder().WithOutputFileName(filepath.Join(t.TempDir(), "recording")).Build()
	defer s.Terminate()
	if s.Monitor() != nil {
		t.Fatal("default simulation unexpectedly has a monitor")
	}
	if err := s.Engine().Run(); err != nil {
		t.Fatalf("running without a monitor: %v", err)
	}
}

func TestHeadlessSimulationPreservesTracing(t *testing.T) {
	s := sim.MakeBuilder().WithOutputFileName(filepath.Join(t.TempDir(), "trace")).
		WithVisTracingOnStart().WithoutSourceRecording().Build()
	if s.Monitor() != nil || !s.VisTracer().IsTracing() {
		t.Fatal("headless simulation must preserve tracing")
	}
	s.VisTracer().StartTask(tracing.TaskStart{ID: 1, Kind: "test", What: "headless", Location: "Agent.test", Time: 1})
	s.VisTracer().EndTask(tracing.TaskEnd{ID: 1, Time: 2})
	s.Terminate()
	db, err := sql.Open("sqlite", s.TraceDBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM trace WHERE What = 'headless'").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("headless recording: count=%d, err=%v", count, err)
	}
	t.Log("No monitor attached; SQLite retained the traced task")
}
