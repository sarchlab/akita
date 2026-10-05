package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a move into flight (req_in open, the
// outstanding Outside-read req_out open), then issues a control Reset and asserts
// the Reset cleanup (ctrlMiddleware.endInflightTasks) ends every started task,
// leaving no started-never-ended tracing leak.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	spec := Definition.DefaultSpec
	spec.BufferSize = 2048
	spec.InsideByteGranularity = 64
	spec.OutsideByteGranularity = 64

	dataMover := Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(Resources{
			InsideMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("InsideMem"),
			},
			OutsideMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("OutsideMem"),
			},
		}).
		WithPorts(makePorts("DataMover", 16, 64, 64, 1024)).
		Build("DataMover")

	for _, p := range allPorts(dataMover) {
		(&ccNoopConn{}).PlugIn(p)
	}

	topPort := dataMover.Ports.Top
	outsidePort := dataMover.Ports.Outside
	ctrlPort := dataMover.Ports.Control

	// Attach the leak tracker BEFORE any traffic is driven so every task start
	// and end is observed.
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(dataMover, rec)

	makeMove := func() messaging.Msg {
		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   64},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Cmd"),
			Dst:          ctrlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

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
