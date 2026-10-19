package rob

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/tracingtest"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
)

// TestResetEndsInflightTracingTasks drives a request into the ROB so its req_in
// and shadow req_out tasks are open, then issues a Reset and asserts both tasks
// are ended — i.e. a mid-flight Reset leaves no started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) {
	sim := akitasim.MakeBuilder().WithTaskTracking().Build()

	spec := DefaultSpec()
	spec.BufferSize = 4
	spec.NumReqPerCycle = 2
	spec.Bottom = messaging.RemotePort("BottomUnit")

	port := func(name string) messaging.Port {
		p := twowaybuffered.NewPort(4, 4)
		return p
	}

	topPort := port("Top")
	ctrlPort := port("Control")

	rob := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		Build("Rob")

	rob.BindPort("Top", topPort)
	rob.BindPort("Bottom", port("Bottom"))
	rob.BindPort("Control", ctrlPort)
	for _, p := range []messaging.Port{rob.Ports.Top, rob.Ports.Bottom, rob.Ports.Control} {
		(&noopConn{}).BindPort(p)
	}
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	rec := &tracingtest.LeakRecorder{}
	rob.TaskTracker().Subscribe(rec, rob)

	// Admit a read: topDown opens the top req_in and the shadow req_out, then
	// waits on the bottom response (which never comes — the request is in flight).
	read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4},
		ID:  sim.NewID(),
		Src: messaging.RemotePort("Agent"),
		Dst: topPort.AsRemote(),
	}

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
		ID:  sim.NewID(),
		Src: messaging.RemotePort("Cmd"),
		Dst: ctrlPort.AsRemote(),
	}

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
