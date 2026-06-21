package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
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

	spec := DefaultSpec()
	spec.BufferSize = 2048
	spec.InsideByteGranularity = 64
	spec.OutsideByteGranularity = 64

	reg := modeling.NewStandaloneRegistrar(engine)

	dataMover := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		WithResources(Resources{
			InsideMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("InsideMem"),
			},
			OutsideMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("OutsideMem"),
			},
		}).
		Build("DataMover")

	assign := func(name string, bufSize int) messaging.Port {
		p := modeling.MakePortBuilder().
			WithRegistrar(reg).
			WithComponent(dataMover).
			WithSpec(modeling.PortSpec{BufSize: bufSize}).
			Build(name)
		dataMover.AssignPort(name, p)
		(&ccNoopConn{}).PlugIn(p)
		return p
	}

	topPort := assign("Top", 16)
	assign("Inside", 64)
	outsidePort := assign("Outside", 64)
	ctrlPort := assign("Control", 1024)

	// Attach the leak tracker BEFORE any traffic is driven so every task start
	// and end is observed.
	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(dataMover, rec)

	makeMove := func() datamoverprotocol.DataMoveRequest {
		req := datamoverprotocol.DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 0
		req.DstSide = "inside"
		req.ByteSize = 64
		req.TrafficClass = "datamoverprotocol.DataMoveRequest"
		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) memcontrolprotocol.Req {
		req := memcontrolprotocol.Req{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Cmd")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
		return req
	}

	// startMove delivers a move and ticks until the first Outside read is issued.
	// The read is left unanswered, so the move's req_in task and the read's
	// req_out task stay open.
	startMove := func() (memprotocol.ReadReq, bool) {
		topPort.Deliver(makeMove())

		var read memprotocol.ReadReq
		gotRead := false
		for i := 0; i < 64 && !gotRead; i++ {
			dataMover.Tick()
			if out := outsidePort.RetrieveOutgoing(); out != nil {
				read, gotRead = out.(memprotocol.ReadReq)
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
		dataMover.Tick()
		if out := ctrlPort.RetrieveOutgoing(); out != nil {
			if rsp, ok := out.(memcontrolprotocol.Rsp); ok &&
				rsp.Command == memcontrolprotocol.CmdReset {
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
