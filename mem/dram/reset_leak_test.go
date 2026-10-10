package dram

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

// TestResetEndsInflightTracingTasks admits a read into the DRAM controller so
// its req_in and sub-trans tasks are open, then issues a Reset before the DRAM
// access latency elapses (the request is still in flight) and asserts both
// kinds of task are ended — i.e. a mid-flight Reset leaves no
// started-never-ended task. DRAM is a leaf, so there is no downstream req_out.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	comp := Definition.Builder().
		WithSimulation(sim).
		WithResources(Resources{Storage: mem.NewStorage(1 * mem.MB)}).
		Build("DRAM")
	compPorts := defaultPorts("DRAM", 16)
	comp.BindPort("Top", compPorts.Top)
	comp.BindPort("Control", compPorts.Control)

	topPort := comp.Ports.Top
	ctrlPort := comp.Ports.Control
	(&noopConn{}).BindPort(topPort)
	(&noopConn{}).BindPort(ctrlPort)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	// Attach the leak tracer BEFORE any traffic, so it sees every task start.
	// Deliberately not CollectIncomingBufferTrace: those buffer tasks are ended
	// by the port retrieve hook, not the component.
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Admit a read: parseTop opens the req_in task and one sub-trans task per
	// sub-transaction, then enqueues commands whose data return is many cycles
	// out. Ticking just enough to admit the transaction — but far fewer than the
	// DRAM access latency — leaves it in flight.
	read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Agent"),
		Dst:          topPort.AsRemote(),
		TrafficBytes: 12,
		TrafficClass: "memprotocol.ReadReq"}

	topPort.Deliver(read)

	for i := 0; i < 8 && len(comp.State.Transactions) == 0; i++ {
		modelingtest.Tick(comp)
	}

	if len(comp.State.Transactions) == 0 {
		t.Fatal("read was not admitted; no in-flight transaction")
	}

	// Confirm the transaction is genuinely still in flight: no sub-transaction
	// has completed yet, so its sub-trans task is open and a leak is possible.
	for _, sub := range comp.State.Transactions[0].SubTransactions {
		if sub.Completed {
			t.Fatal("sub-transaction completed before reset; not in flight")
		}
	}

	// Guard against a vacuous test: req_in + at least one sub-trans must be open.
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and at least one sub-trans to be opened, "+
			"got %d task starts", rec.NumStarted())
	}

	// Reset while the transaction is in flight.
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
