package rob

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a request into the ROB so its req_in
// and shadow req_out tasks are open, then issues a Reset and asserts both tasks
// are ended — i.e. a mid-flight Reset leaves no started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)

	spec := DefaultSpec()
	spec.BufferSize = 4
	spec.NumReqPerCycle = 2
	spec.BottomUnit = messaging.RemotePort("BottomUnit")

	rob := MakeBuilder().WithRegistrar(reg).WithSpec(spec).Build("Rob")

	assign := func(name string) messaging.Port {
		p := modeling.MakePortBuilder().
			WithRegistrar(reg).
			WithComponent(rob).
			WithSpec(modeling.PortSpec{BufSize: 4}).
			Build(name)
		rob.AssignPort(name, p)
		(&noopConn{}).PlugIn(p)
		return p
	}

	topPort := assign("Top")
	assign("Bottom")
	ctrlPort := assign("Control")

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(rob, rec)

	// Admit a read: topDown opens the top req_in and the shadow req_out, then
	// waits on the bottom response (which never comes — the request is in flight).
	read := memprotocol.ReadReq{Address: 0, AccessByteSize: 4}
	read.ID = timing.GetIDGenerator().Generate()
	read.Src = messaging.RemotePort("Agent")
	read.Dst = topPort.AsRemote()
	read.TrafficClass = "memprotocol.ReadReq"
	topPort.Deliver(read)
	rob.Tick()

	if len(rob.State.Transactions) != 1 {
		t.Fatalf("expected 1 in-flight transaction, got %d",
			len(rob.State.Transactions))
	}
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and req_out to be opened, got %d task starts",
			rec.NumStarted())
	}

	// Reset while the transaction is in flight.
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = timing.GetIDGenerator().Generate()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = ctrlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	ctrlPort.Deliver(reset)

	acked := false
	for range 16 {
		rob.Tick()
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
