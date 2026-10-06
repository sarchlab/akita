package rob

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
	"github.com/sarchlab/akita/v5/sim/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a request into the ROB so its req_in
// and shadow req_out tasks are open, then issues a Reset and asserts both tasks
// are ended — i.e. a mid-flight Reset leaves no started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	spec := Definition.DefaultSpec
	spec.BufferSize = 4
	spec.NumReqPerCycle = 2
	spec.BottomUnit = messaging.RemotePort("BottomUnit")

	port := func(name string) messaging.Port {
		p := messaging.NewPort("Rob."+name, 4, 4)
		(&noopConn{}).PlugIn(p)
		return p
	}

	topPort := port("Top")
	ctrlPort := port("Control")

	rob := Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithPorts(Ports{Top: topPort, Bottom: port("Bottom"), Control: ctrlPort}).
		Build("Rob")

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(rob, rec)

	// Admit a read: topDown opens the top req_in and the shadow req_out, then
	// waits on the bottom response (which never comes — the request is in flight).
	read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Agent"),
		Dst:          topPort.AsRemote(),
		TrafficClass: "memprotocol.ReadReq"}

	topPort.Deliver(read)
	modelingtest.Tick(rob)

	if len(rob.State.Transactions) != 1 {
		t.Fatalf("expected 1 in-flight transaction, got %d",
			len(rob.State.Transactions))
	}
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and req_out to be opened, got %d task starts",
			rec.NumStarted())
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
		modelingtest.Tick(rob)
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
