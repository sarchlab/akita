package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// milestoneRecorder captures the task starts and milestones the MMU emits, in
// emission order and without the DBTracer's dedup, so a test can assert which
// task (incoming-buffer vs req_in) each admission milestone is attributed to.
type milestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	milestones []tasks.Milestone
}

func (r *milestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *milestoneRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

func (r *milestoneRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

// milestoneClock lets manual middleware ticks observe nonzero waits without
// running the event queue, which would also admit or retire test traffic.
type milestoneClock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

func (c *milestoneClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

func TestMMUMilestones(t *testing.T) {
	var (
		engine    timing.Engine
		sim       *testsim.Simulation
		pageTable *vm.PageTable
		mmuComp   *Comp
		topPort   messaging.Port
		mw        *translationMW
		rec       *milestoneRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)
		pageTable = vm.NewPageTable(12)

		mmuComp = modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			Build("MMU")
		mmuCompPorts := makePorts(sim, "MMU", 16)
		mmuComp.BindPort("Top", mmuCompPorts.Top)
		mmuComp.BindPort("Control", mmuCompPorts.Control)

		topPort = mmuComp.Ports.Top

		(&noopConn{}).BindPort(topPort)
		(&noopConn{}).BindPort(mmuComp.Ports.Control)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		mw = mmuComp.Middlewares.Translation

		// Attach the recorder before driving so buffer enqueue allocates
		// real task IDs (it returns 0 when there are no hooks).
		rec = &milestoneRecorder{}
		mmuComp.TaskTracker().Subscribe(rec, mmuComp)
		// The admission milestone lands on the Top-port buffer task, so the test
		// needs the buffer task to exist.
	}

	makeReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 0},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent.Top"),
			Dst: topPort.AsRemote(),
		}

		return req
	}

	t.Run("emits a hardware_resource admission milestone on the buffer task "+
		"when it admits the head request", func(t *testing.T) {
		setup1(t)

		req := makeReq(0x1000)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		require.True(t, mw.parseFromTop())

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		require.NotZero(t, bufID)
		require.NotZero(t, reqInID)
		require.NotEqual(t, reqInID, bufID)

		// The buffer task owns the at-head admission wait: reached the head of
		// the Top buffer, then waited for a free walk slot.
		bufMs := rec.milestonesOn(bufID)
		require.Len(t, bufMs, 1)

		require.Equal(t, tasks.MilestoneKindHardwareResource, bufMs[0].Kind)
		require.Equal(t, mmuComp.Name()+".walk", bufMs[0].What)
	})

	t.Run("labels the local page-walk latency as work on the req_in when the "+
		"walk completes", func(t *testing.T) {
		setup1(t)

		// The page must already be mapped: with AutoPageAllocation off (the
		// default), finalizePageWalk panics on a missing page.
		pageTable.Insert(vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x5000,
			PageSize: 1 << 12,
			Valid:    true,
		})

		req := makeReq(0x1000)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		// Admit the request (opens req_in and starts the walk), then drive the
		// CycleLeft countdown to completion so doPageWalkHit sends the response
		// and emits the walk milestone.
		require.True(t, mw.parseFromTop())

		sent := false
		for i := 0; i < 64 && !sent; i++ {
			mw.Handle(modelingtest.TickEvent(mmuComp))
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(addrtransprotocol.Rsp); ok {
					sent = true
				}
			}
		}
		require.True(t, sent)

		// The walk milestone lands on the req_in (walking.RecvTaskID ==
		// the stored RecvTaskID), labelling the local-walk latency as work.
		reqInID := rec.taskID("req_in")
		require.NotZero(t, reqInID)

		reqInMs := rec.milestonesOn(reqInID)
		var workMs *tasks.Milestone
		for i := range reqInMs {
			if reqInMs[i].Kind == tasks.MilestoneKindWork {
				workMs = &reqInMs[i]
				break
			}
		}
		require.NotNil(t, workMs)
		require.Equal(t, mmuComp.Name()+".walk", workMs.What)
	})

	t.Run("does not admit (and emits no admission milestone) while servicing "+
		"the max in-flight requests", func(t *testing.T) {
		setup1(t)

		mmuComp.State = state{
			WalkingTranslations: make([]transactionState, 16),
		}

		req := makeReq(0x1000)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		require.False(t, mw.parseFromTop())

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		require.NotZero(t, bufID)
		// No queue delay and no admission milestone: the request is still blocked.
		bufMs := rec.milestonesOn(bufID)
		require.Empty(t, bufMs)
	})
}
