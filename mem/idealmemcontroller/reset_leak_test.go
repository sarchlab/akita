package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
	"github.com/sarchlab/akita/v5/sim/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a read into the ideal memory
// controller so its req_in task is open and the access is mid-flight (admitted
// into InflightTransactions but not yet completed, because fewer ticks elapse
// than the access latency), then issues a Reset and asserts the req_in task is
// ended — i.e. a mid-flight Reset leaves no started-never-ended task. The
// controller is a leaf, so the only open task per access is its req_in.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	storage := mem.NewStorage(1 * mem.MB)

	spec := Definition.DefaultSpec
	spec.Width = 4
	// A long latency keeps the access in flight: one Tick admits it into
	// InflightTransactions (opening req_in) but is far short of completing it.
	spec.Latency = 100
	spec.CacheLineSize = 64

	comp := Definition.Builder().
		WithSimulation(sim).
		WithResources(Resources{Storage: storage}).
		WithSpec(spec).
		Build("MemCtrl")
	compPorts := makePorts("MemCtrl", 16)
	comp.BindPort("Top", compPorts.Top)
	comp.BindPort("Control", compPorts.Control)

	topPort := comp.Ports.Top
	ctrlPort := comp.Ports.Control
	for _, p := range []messaging.Port{topPort, ctrlPort} {
		(&noopConn{}).BindPort(p)
	}
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Admit a read: takeNewReqs opens the req_in task and parks the access in
	// InflightTransactions; the access never completes within the ticks below.
	read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Agent"),
		Dst:          topPort.AsRemote(),
		TrafficClass: "memprotocol.ReadReq"}

	topPort.Deliver(read)
	modelingtest.Tick(comp)

	if len(comp.State.InflightTransactions) < 1 {
		t.Fatalf("expected the read to be in flight, got %d in-flight transactions",
			len(comp.State.InflightTransactions))
	}
	if rec.NumStarted() < 1 {
		t.Fatalf("expected req_in to be opened, got %d task starts",
			rec.NumStarted())
	}

	// Reset while the access is in flight.
	reset := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Cmd"),
		Dst:          ctrlPort.AsRemote(),
		TrafficClass: "memcontrolprotocol.Req"}

	ctrlPort.Deliver(reset)

	acked := false
	for range 16 {
		modelingtest.Tick(comp)
		if msg, ok := ctrlPort.RetrieveOutgoing(); ok {
			if rsp, ok := msg.Payload.(memcontrolprotocol.Rsp); ok && rsp.
				Command == memcontrolprotocol.CmdReset {
				acked = true
				break
			}
		}
	}

	if !acked {
		t.Fatal("Reset was not acked")
	}
	if open := rec.OpenTasks(); len(open) != 0 {
		t.Errorf("Reset left %d tracing task(s) unended: %s",
			len(open), rec.OpenSummary())
	}
}
