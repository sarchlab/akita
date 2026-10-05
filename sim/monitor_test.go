package sim_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type lifecycleMonitor struct {
	t         *testing.T
	events    []string
	runtime   timing.Simulation
	tracer    *tracing.DBTracer
	path      string
	component naming.Named
	port      sim.Port
}

func (m *lifecycleMonitor) Start(sim timing.Simulation, tracer *tracing.DBTracer, path string) {
	m.events = append(m.events, "start")
	m.runtime, m.tracer, m.path = sim, tracer, path
	if sim.Engine() == nil || tracer == nil {
		m.t.Fatal("monitor started before runtime services were ready")
	}
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
	recorder := m.runtime.(*sim.Simulation).DataRecorder()
	recorder.CreateTable("monitor_shutdown", struct{ Stopped bool }{})
	recorder.InsertData("monitor_shutdown", struct{ Stopped bool }{true})
	recorder.Flush()
}

type namedComponent string

func (c namedComponent) Name() string { return string(c) }

func TestInjectedMonitorLifecycle(t *testing.T) {
	output := filepath.Join(t.TempDir(), "recording")
	monitor := &lifecycleMonitor{t: t}
	sim := sim.MakeBuilder().WithOutputFileName(output).WithMonitor(monitor).Build()
	if sim.Monitor() != monitor || monitor.runtime != sim || monitor.path != output+".sqlite3" {
		t.Fatal("monitor did not receive the simulation and recording destination")
	}
	component := namedComponent("Agent")
	port := messaging.NewPort("Agent.Top", 2, 2)
	sim.RegisterComponent(component)
	sim.RegisterPort(port)
	sim.Terminate()
	if monitor.component != component || monitor.port != port {
		t.Fatal("monitor did not receive registered component and port")
	}
	want := []string{"start", "component", "port", "stop"}
	if !reflect.DeepEqual(monitor.events, want) {
		t.Fatalf("lifecycle = %v, want %v", monitor.events, want)
	}
}

func TestMonitoringIsOptIn(t *testing.T) {
	sim := sim.MakeBuilder().WithOutputFileName(filepath.Join(t.TempDir(), "recording")).Build()
	defer sim.Terminate()
	if sim.Monitor() != nil {
		t.Fatal("default simulation unexpectedly has a monitor")
	}
	if err := sim.Engine().Run(); err != nil {
		t.Fatalf("running without a monitor: %v", err)
	}
}
