package sim_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/internal/assembly"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

type lifecycleMonitor struct {
	events     []string
	runtime    *sim.Simulation
	path       string
	components []sim.Component
	ports      []messaging.Port
}

func (m *lifecycleMonitor) Start(s *sim.Simulation) {
	if s.Engine() == nil || s.VisTracer() == nil || s.DataRecorder() == nil {
		panic("monitor started before runtime services were ready")
	}
	m.runtime, m.path = s, s.TraceDBPath()
	m.components, m.ports = s.Components(), s.Ports()
	m.events = append(m.events, "start")
}

func (m *lifecycleMonitor) Stop() {
	m.events = append(m.events, "stop")
	// Monitors stop while the recording is still usable.
	recorder := m.runtime.DataRecorder()
	recorder.CreateTable("monitor_shutdown", struct{ Stopped bool }{})
	recorder.InsertData("monitor_shutdown", struct{ Stopped bool }{true})
	recorder.Flush()
}

type initializedComponent struct {
	assembly.Member
	ready, invalid bool
}

func (*initializedComponent) Name() string { return "Agent" }
func (c *initializedComponent) validateSetup() error {
	if c.invalid {
		return errors.New("invalid setup")
	}
	return nil
}
func (c *initializedComponent) initializeComponent() { c.ready = true }

func (c *initializedComponent) BindPort(slot string, p messaging.Port) {
	p.BindOwner(c, c.Name()+"."+slot)
}
func (*initializedComponent) NotifyRecv(messaging.Port)     {}
func (*initializedComponent) NotifyPortFree(messaging.Port) {}

func TestInjectedMonitorLifecycle(t *testing.T) {
	output := filepath.Join(t.TempDir(), "recording.sqlite3")
	monitor := &lifecycleMonitor{}
	s := sim.MakeBuilder().WithOutputPath(output).WithMonitor(monitor).Build()
	component := &initializedComponent{invalid: true}
	component.Member = assembly.NewMember(s, component.validateSetup, component.initializeComponent)
	port := twowaybuffered.MakeBuilder().
		WithSimulation(s).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 2, OutgoingCapacity: 3}).
		Build()
	port.BindOwner(monitorPortOwner{}, "Agent.Port")
	s.RegisterComponent(component)
	s.RegisterPort(port)
	if monitor.runtime != nil {
		t.Fatal("monitor started before initialization")
	}
	if err := s.Initialize(); err == nil {
		t.Fatal("invalid setup was accepted")
	}
	if monitor.runtime != nil {
		t.Fatal("failed initialization started monitor")
	}
	component.invalid = false
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	if !component.ready || monitor.runtime != s || monitor.path != output ||
		len(monitor.components) != 1 || monitor.components[0] != component ||
		len(monitor.ports) != 1 || monitor.ports[0] != port {
		t.Fatal("monitor did not discover initialized objects")
	}
	// The inventories expose live objects but cannot mutate registry slices.
	monitor.components[0], monitor.ports[0] = nil, nil
	if s.Components()[0] != component || s.Ports()[0] != port {
		t.Fatal("inventory aliases registry")
	}
	if err := s.Initialize(); err == nil {
		t.Fatal("repeated initialization accepted")
	}
	s.Terminate()
	if len(monitor.events) != 2 || monitor.events[0] != "start" || monitor.events[1] != "stop" {
		t.Fatalf("unexpected lifecycle: %v", monitor.events)
	}
	t.Log("Monitor started once after state initialization, discovered complete copied inventories, " +
		"and stopped before recording closed")
}

func TestUninitializedMonitorIsNotStopped(t *testing.T) {
	m := &lifecycleMonitor{}
	s := sim.MakeBuilder().WithMonitor(m).WithOutputPath(filepath.Join(t.TempDir(), "recording.sqlite3")).Build()
	s.Terminate()
	if len(m.events) != 0 {
		t.Fatalf("unstarted monitor received lifecycle calls: %v", m.events)
	}
}

func TestMonitoringIsOptIn(t *testing.T) {
	s := sim.MakeBuilder().WithOutputPath(filepath.Join(t.TempDir(), "recording") + ".sqlite3").Build()
	defer s.Terminate()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := s.Engine().Run(); err != nil {
		t.Fatalf("running without a monitor: %v", err)
	}
}

func TestHeadlessSimulationPreservesTracing(t *testing.T) {
	s := sim.MakeBuilder().WithOutputPath(filepath.Join(t.TempDir(), "trace") + ".sqlite3").
		WithVisTracingOnStart().WithoutSourceRecording().Build()
	if !s.VisTracer().IsTracing() {
		t.Fatal("headless simulation must preserve tracing")
	}
	s.VisTracer().TaskStarted(tasks.Task{ID: 1, Kind: "test", What: "headless", Location: "Agent.test", StartTime: 1})
	s.VisTracer().TaskEnded(tasks.Task{ID: 1, Kind: "test", What: "headless",
		Location: "Agent.test", StartTime: 1, EndTime: 2})
	s.Terminate()
	db, err := sql.Open("sqlite", s.TraceDBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM akita$trace WHERE What = 'headless'").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("headless recording: count=%d, err=%v", count, err)
	}
	t.Log("No monitor attached; SQLite retained the traced task")
}

type monitorPortOwner struct{}

func (monitorPortOwner) NotifyRecv(messaging.Port)     {}
func (monitorPortOwner) NotifyPortFree(messaging.Port) {}

func (o monitorPortOwner) Name() string                           { return "MonitorPortOwner" }
func (o monitorPortOwner) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
