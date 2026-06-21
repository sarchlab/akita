package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a read into a bank pipeline so its
// req_in and pipeline subtask are open but not yet completed, then issues a
// Reset and asserts both tasks are ended — i.e. a mid-flight Reset that rebuilds
// the banks leaves no started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	storage := mem.NewStorage(1 * mem.MB)

	reg := modeling.NewStandaloneRegistrar(engine)
	comp := MakeBuilder().
		WithRegistrar(reg).
		WithResources(Resources{Storage: storage}).
		Build("BankedMem")

	assignPort(reg, comp, "Top", 16)
	assignPort(reg, comp, "Control", 16)

	topPort := comp.GetPortByName("Top")
	ctrlPort := comp.GetPortByName("Control")
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
	read := makeReadReq(messaging.RemotePort("Agent"), topPort.AsRemote(), 0)
	topPort.Deliver(read)
	comp.Tick()

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
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = timing.GetIDGenerator().Generate()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = ctrlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	ctrlPort.Deliver(reset)

	acked := false
	for range 16 {
		comp.Tick()
		if msg := ctrlPort.RetrieveOutgoing(); msg != nil {
			if rsp, ok := msg.(memcontrolprotocol.Rsp); ok &&
				rsp.Command == memcontrolprotocol.CmdReset {
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
