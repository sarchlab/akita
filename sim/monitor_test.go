package sim_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
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

func (m *lifecycleMonitor) Start() {
	m.events = append(m.events, "start")
}

func newLifecycleMonitor(t *testing.T, s *sim.Simulation) *lifecycleMonitor {
	t.Helper()
	if s.Engine() == nil || s.VisTracer() == nil || s.DataRecorder() == nil {
		t.Fatal("factory called before runtime services were ready")
	}
	return &lifecycleMonitor{runtime: s, path: s.TraceDBPath()}
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

func TestMonitorFactoryLifecycle(t *testing.T) {
	output := filepath.Join(t.TempDir(), "recording")
	var monitor *lifecycleMonitor
	s := sim.MakeBuilder().WithOutputFileName(output).
		WithMonitorFactory(func(s *sim.Simulation) sim.Monitor {
			monitor = newLifecycleMonitor(t, s)
			return monitor
		}).Build()
	if s.Monitor() != monitor || monitor.runtime != s || monitor.path != output+".sqlite3" {
		t.Fatal("monitor did not receive the simulation and recording destination")
	}
	component := namedComponent("Agent")
	port := messaging.NewPort("Agent.Top", 2, 2)
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

func TestWithoutMonitoringSkipsFactoryAndPreservesTracing(t *testing.T) {
	factory := func(*sim.Simulation) sim.Monitor {
		t.Fatal("disabled monitor factory was invoked")
		return nil
	}
	for _, disableFirst := range []bool{false, true} {
		b := sim.MakeBuilder().WithOutputFileName(filepath.Join(t.TempDir(), "trace")).
			WithVisTracingOnStart().WithoutSourceRecording()
		if disableFirst {
			b = b.WithoutMonitoring().WithMonitorFactory(factory)
		} else {
			b = b.WithMonitorFactory(factory).WithoutMonitoring()
		}
		s := b.Build()
		if s.Monitor() != nil || !s.VisTracer().IsTracing() {
			t.Fatal("WithoutMonitoring must disable monitoring but preserve tracing")
		}
		s.VisTracer().StartTask(tracing.TaskStart{ID: 1, Kind: "test", What: "headless", Location: "Agent.test", Time: 1})
		s.VisTracer().EndTask(tracing.TaskEnd{ID: 1, Time: 2})
		s.Terminate()
		db, err := sql.Open("sqlite", s.TraceDBPath())
		if err != nil {
			t.Fatal(err)
		}
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM trace WHERE What = 'headless'").Scan(&count)
		db.Close()
		if err != nil || count != 1 {
			t.Fatalf("headless recording: count=%d, err=%v", count, err)
		}
		t.Log("WithoutMonitoring skipped factory construction; SQLite retained the traced task")
	}
}

func TestReusedBuilderCreatesFreshMonitors(t *testing.T) {
	b := sim.MakeBuilder().WithMonitorFactory(func(s *sim.Simulation) sim.Monitor {
		return newLifecycleMonitor(t, s)
	})
	first := b.WithOutputFileName(filepath.Join(t.TempDir(), "first")).Build()
	defer first.Terminate()
	second := b.WithOutputFileName(filepath.Join(t.TempDir(), "second")).Build()
	defer second.Terminate()
	if first.Monitor() == second.Monitor() {
		t.Fatal("builder reused a monitor across simulations")
	}
	if first.Monitor().(*lifecycleMonitor).runtime != first || second.Monitor().(*lifecycleMonitor).runtime != second {
		t.Fatal("monitor is bound to the wrong simulation")
	}
}
