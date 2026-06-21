package idealmemcontroller

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a read into the ideal memory
// controller so its req_in task is open and the access is mid-flight (admitted
// into InflightTransactions but not yet completed, because fewer ticks elapse
// than the access latency), then issues a Reset and asserts the req_in task is
// ended — i.e. a mid-flight Reset leaves no started-never-ended task. The
// controller is a leaf, so the only open task per access is its req_in.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)

	storage := mem.NewStorage(1 * mem.MB)

	spec := DefaultSpec()
	spec.Width = 4
	// A long latency keeps the access in flight: one Tick admits it into
	// InflightTransactions (opening req_in) but is far short of completing it.
	spec.Latency = 100
	spec.CacheLineSize = 64

	comp := MakeBuilder().
		WithRegistrar(reg).
		WithResources(Resources{Storage: storage}).
		WithSpec(spec).
		Build("MemCtrl")

	comp.AssignPort("Top",
		messaging.NewPort(comp, 16, 16, comp.Name()+".Top"))
	comp.AssignPort("Control",
		messaging.NewPort(comp, 16, 16, comp.Name()+".Control"))

	topPort := comp.GetPortByName("Top")
	ctrlPort := comp.GetPortByName("Control")
	for _, p := range []messaging.Port{topPort, ctrlPort} {
		(&noopConn{}).PlugIn(p)
	}

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Admit a read: takeNewReqs opens the req_in task and parks the access in
	// InflightTransactions; the access never completes within the ticks below.
	read := memprotocol.ReadReq{Address: 0, AccessByteSize: 4}
	read.ID = timing.GetIDGenerator().Generate()
	read.Src = messaging.RemotePort("Agent")
	read.Dst = topPort.AsRemote()
	read.TrafficClass = "memprotocol.ReadReq"
	topPort.Deliver(read)
	comp.Tick()

	if len(comp.State.InflightTransactions) < 1 {
		t.Fatalf("expected the read to be in flight, got %d in-flight transactions",
			len(comp.State.InflightTransactions))
	}
	if rec.NumStarted() < 1 {
		t.Fatalf("expected req_in to be opened, got %d task starts",
			rec.NumStarted())
	}

	// Reset while the access is in flight.
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
