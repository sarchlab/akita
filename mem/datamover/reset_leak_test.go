package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/tracingtest"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
)

// TestResetEndsInflightTracingTasks drives a move into flight (req_in open, the
// outstanding Outside-read req_out open), then issues a control Reset and asserts
// the Reset cleanup (ctrlMiddleware.endInflightTasks) ends every started task,
// leaving no started-never-ended tracing leak.
func TestResetEndsInflightTracingTasks(t *testing.T) {
	sim := akitasim.MakeBuilder().WithTaskTracking().Build()

	spec := DefaultSpec()
	spec.BufferSize = 2048
	spec.InsideByteGranularity = 64
	spec.OutsideByteGranularity = 64

	spec.Inside.Destinations = []messaging.RemotePort{messaging.RemotePort("InsideMem")}

	spec.Outside.Destinations = []messaging.RemotePort{messaging.RemotePort("OutsideMem")}
	dataMover := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		Build("DataMover")
	dataMoverPorts := makePorts(sim, "DataMover", 16, 64, 64, 1024)
	dataMover.BindPort("Top", dataMoverPorts.Top)
	dataMover.BindPort("Inside", dataMoverPorts.Inside)
	dataMover.BindPort("Outside", dataMoverPorts.Outside)
	dataMover.BindPort("Control", dataMoverPorts.Control)

	for _, p := range allPorts(dataMover) {
		(&ccNoopConn{}).BindPort(p)
	}
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	topPort := dataMover.Ports.Top
	outsidePort := dataMover.Ports.Outside
	ctrlPort := dataMover.Ports.Control

	// Attach the leak tracker BEFORE any traffic is driven so every task start
	// and end is observed.
	rec := &tracingtest.LeakRecorder{}
	dataMover.TaskTracker().Subscribe(rec, dataMover)

	makeMove := func() messaging.Msg {
		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   64},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),
		}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Cmd"),
			Dst: ctrlPort.AsRemote(),
		}

		return req
	}

	// startMove delivers a move and ticks until the first Outside read is issued.
	// The read is left unanswered, so the move's req_in task and the read's
	// req_out task stay open.
	startMove := func() (messaging.Msg, bool) {
		topPort.Deliver(makeMove())

		var read messaging.Msg
		gotRead := false
		for i := 0; i < 64 && !gotRead; i++ {
			modelingtest.Tick(dataMover)
			if out, ok := outsidePort.RetrieveOutgoing(); ok {
				_, gotRead = out.Payload.(memprotocol.ReadReq)
				read = out
			}
		}
		return read, gotRead
	}

	_, gotRead := startMove()
	if !gotRead {
		t.Fatal("expected an Outside read to be issued")
	}
	if !dataMover.State.CurrentTransaction.Active {
		t.Fatal("expected the move to be active in flight")
	}

	// Guard against a vacuous test: the req_in and the read req_out must be open.
	if rec.NumStarted() < 2 {
		t.Fatalf("expected at least 2 tasks opened, got %d", rec.NumStarted())
	}

	reset := makeCtrlReq(memcontrolprotocol.CmdReset)
	ctrlPort.Deliver(reset)

	acked := false
	for i := 0; i < 64 && !acked; i++ {
		modelingtest.Tick(dataMover)
		if out, ok := ctrlPort.RetrieveOutgoing(); ok {
			if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
				Command == memcontrolprotocol.CmdReset {
				acked = true
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
