package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
	"github.com/sarchlab/akita/v5/sim/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a read into a bank pipeline so its
// req_in and pipeline subtask are open but not yet completed, then issues a
// Reset and asserts both tasks are ended — i.e. a mid-flight Reset that rebuilds
// the banks leaves no started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	storage := mem.NewStorage(1 * mem.MB)

	comp := Definition.Builder().
		WithSimulation(sim).
		WithResources(Resources{Storage: storage}).
		WithPorts(makePorts("BankedMem", 16, 16)).
		Build("BankedMem")

	topPort := comp.Ports.Top
	ctrlPort := comp.Ports.Control
	for _, p := range []messaging.Port{topPort, ctrlPort} {
		(&noopConn{}).PlugIn(p)
	}

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Admit one read. A single Tick lets dispatchMW route it into the bank
	// pipeline, opening req_in (TraceReqReceive) and the pipeline subtask. The
	// pipeline latency (BankPipelineDepth*StageLatency = 10) far exceeds one
	// tick, so the item is still in flight: it has not reached PostPipelineBuf
	// and so has not been finalized/completed. The memory is a leaf, so there is
	// no downstream req_out.
	read := makeReadReq(sim, messaging.RemotePort("Agent"), topPort.AsRemote(), 0)
	topPort.Deliver(read)
	modelingtest.Tick(comp)

	if bankIsQuiescent(&comp.State.Banks[0]) {
		t.Fatal("expected the read to be in flight in bank 0, but bank is quiescent")
	}
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and pipeline subtask to be opened, "+
			"got %d task starts", rec.NumStarted())
	}
	if open := rec.OpenTasks(); len(open) < 2 {
		t.Fatalf("expected at least 2 open tasks before reset, got %d: %s",
			len(open), rec.OpenSummary())
	}

	// Reset while the read is in flight.
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
