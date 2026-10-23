package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/tracingtest"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

// TestResetEndsInflightTracingTasks drives a read miss into the writethrough
// cache so the transaction has both of its tracing tasks open — the req_in for
// the top request and the downstream req_out for the bottom fetch — then issues
// a Reset and asserts every task is ended. A mid-flight Reset drops the
// transaction table, so each started task must be ended by endInflightTasks or
// it leaks.
func TestResetEndsInflightTracingTasks(t *testing.T) {
	sim := akitasim.MakeBuilder().WithTaskTracking().Build()

	storage := mem.NewStorage(4 * mem.GB)

	spec := DefaultSpec()
	spec.NumReqPerCycle = 1

	spec.DirectoryWidth = 1
	spec.BankWidth = 1
	spec.NumBanks = 1
	spec.NumMSHREntries = 8
	spec.NumWays = 2
	spec.Log2BlockSize = 6
	spec.BankLatency = 1
	spec.DirLatency = 1
	spec.NumSets = 512
	spec.MaxInFlightRequests = 16

	spec.Bottom.Destinations = []messaging.RemotePort{messaging.RemotePort("LowerCache")}
	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(Resources{Storage: storage}).
		Build("L1Cache")
	compPorts := makePorts(sim, "L1Cache", 16)
	comp.BindPort("Top", compPorts.Top)
	comp.BindPort("Bottom", compPorts.Bottom)
	comp.BindPort("Control", compPorts.Control)

	// Plug each port into a no-op connection before the component is ticked.
	topPort := comp.Ports.Top
	bottomPort := comp.Ports.Bottom
	ctrlPort := comp.Ports.Control
	for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
		(&ccNoopConn{}).BindPort(p)
	}
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	rec := &tracingtest.LeakRecorder{}
	comp.TaskTracker().Subscribe(rec, comp)

	// Admit a read miss. intake opens the req_in; the directory then issues a
	// bottom fetch (req_out) that is never answered, leaving the transaction in
	// flight with both its req_in and req_out tracing tasks open.
	read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Agent"),
		Dst:          topPort.AsRemote(),
		TrafficBytes: 12,
	}

	topPort.Deliver(read)

	// Tick until the bottom fetch has been sent, so the in-flight transaction
	// has HasReadToBottom set and its req_out task is open. Do NOT answer the
	// bottom request.
	bottomSent := false
	for i := 0; i < 256 && !bottomSent; i++ {
		modelingtest.Tick(comp)
		if out, ok := bottomPort.RetrieveOutgoing(); ok {
			if _, ok := out.Payload.(memprotocol.ReadReq); ok {
				bottomSent = true
			}
		}
	}

	if !bottomSent {
		t.Fatal("bottom fetch was never issued; transaction not in flight")
	}

	// The transaction is genuinely in flight: a slot exists that is not yet
	// removed and has issued its downstream read.
	inflight := false
	for i := range comp.State.Transactions {
		trans := &comp.State.Transactions[i]
		if !trans.Removed && trans.HasReadToBottom {
			inflight = true
		}
	}
	if !inflight {
		t.Fatal("expected an in-flight transaction with a bottom read")
	}

	// Before the Reset the transaction's req_in and its downstream req_out are
	// open (the dir_pipeline subtask already closed when the fetch was issued).
	if open := rec.OpenTasks(); len(open) < 2 {
		t.Fatalf("expected req_in and req_out to be open, got %d: %s",
			len(open), rec.OpenSummary())
	}

	// Reset while the transaction is in flight.
	reset := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
		ID:  sim.NewID(),
		Src: messaging.RemotePort("Cmd"),
		Dst: ctrlPort.AsRemote(),
	}

	ctrlPort.Deliver(reset)

	acked := false
	for i := 0; i < 64 && !acked; i++ {
		modelingtest.Tick(comp)
		if msg, ok := ctrlPort.RetrieveOutgoing(); ok {
			if rsp, ok := msg.Payload.(memcontrolprotocol.Rsp); ok && rsp.
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

type resetEndCounter struct {
	tasks.NopObserver
	ends map[uint64]int
}

func (r *resetEndCounter) TaskEnded(task tasks.Task) { r.ends[task.ID]++ }

func TestResetDoesNotEndCompletedReceiveAgain(t *testing.T) {
	spec := DefaultSpec()
	spec.Bottom.Destinations = []messaging.RemotePort{"Lower"}
	sim := akitasim.MakeBuilder().WithTaskTracking().Build()
	pipeline := buildStageTestCache(sim, spec, Resources{
		Storage: mem.NewStorage(1 * mem.MB),
	}, makePorts(sim, "Cache", 8), state{})
	comp := pipeline.comp
	rec := &resetEndCounter{ends: make(map[uint64]int)}
	comp.TaskTracker().Subscribe(rec, comp)
	completed := comp.TaskTracker().StartReqIn(comp, messaging.Msg{ID: comp.NewID()})
	active := comp.TaskTracker().StartReqIn(comp, messaging.Msg{ID: comp.NewID()})
	write := comp.TaskTracker().StartReqOut(comp, messaging.Msg{ID: comp.NewID()}, completed)
	comp.TaskTracker().EndReqIn(completed)
	comp.State.Transactions = []transactionState{
		{HasRead: true, RecvTaskID: completed, Removed: true, HasWriteToBottom: true, WriteTaskID: write},
		{HasWrite: true, RecvTaskID: active},
	}
	(&ctrlMiddleware{comp: comp}).endInflightTasks()
	for _, id := range []uint64{completed, active, write} {
		if rec.ends[id] != 1 {
			t.Fatalf("task %d ended %d times, want once", id, rec.ends[id])
		}
	}
	t.Log("reset ended live receive/write tasks once; completed receive was not ended again")
}
