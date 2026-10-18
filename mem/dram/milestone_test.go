package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/stretchr/testify/require"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// milestoneRecorder captures the task starts and milestones the DRAM emits, in
// emission order and without the DBTracer's same-key/same-time dedup, so a test
// can assert the blocking reasons and the task (buffer vs sub-trans) each one is
// attributed to.
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

func TestDRAMAdmissionMilestones(t *testing.T) {
	var (
		engine  timing.Engine
		sim     timing.Simulation
		memCtrl *Comp
		topPort messaging.Port
		rec     *milestoneRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)

		memCtrl = buildDRAM(sim, DefaultSpec(), "MemCtrl", 16)
		require.NoError(t, sim.Initialize())

		topPort = memCtrl.Ports.Top

		// Subscribe before driving to retain the starts of request and buffer tasks.
		rec = &milestoneRecorder{}
		memCtrl.TaskTracker().Subscribe(rec, memCtrl)
	}

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Agent"),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	t.Run("records a SubTransQueue admission milestone on the buffer task", func(t *testing.T) {
		setup1(t)

		topPort.Deliver(makeRead(0x40))
		engine.(*milestoneClock).now += 1000

		// One tick runs parseTop, which admits the request: the buffer task's
		// queue admission milestone is emitted just before RetrieveIncoming.
		modelingtest.Tick(memCtrl)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		require.NotZero(t, bufID)

		bufMs := rec.milestonesOn(bufID)
		// No queue delay: the buffer task carries only the SubTransQueue admission milestone.
		require.Len(t, bufMs, 1)

		require.Equal(t, tasks.MilestoneKindQueue, bufMs[0].Kind)
		require.Equal(t, memCtrl.Name()+".SubTransQueue", bufMs[0].What)
	})
}

func TestDRAMRefreshStallAttribution(t *testing.T) {
	t.Run("charges a refresh milestone to the sub-trans that issues "+
		"after a refresh window", func(t *testing.T) {
		engine := &milestoneClock{Engine: timing.NewSerialEngine()}
		sim := testsim.New(engine, true)

		// A short tREFI/tRFC so a refresh window opens quickly and the request
		// that arrives during it is the one charged the stall.
		spec := DefaultSpec()
		spec.TREFI = 1
		spec.TRFC = 3

		memCtrl := buildDRAM(sim, spec, "MemCtrl", 16)
		require.NoError(t, sim.Initialize())

		topPort := memCtrl.Ports.Top

		rec := &milestoneRecorder{}
		memCtrl.TaskTracker().Subscribe(rec, memCtrl)

		read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0x40, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Agent"),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		topPort.Deliver(read)

		engine.now += 1000

		// Tick enough for the refresh window to open and close and the command
		// to finally issue.
		for i := 0; i < 20; i++ {
			modelingtest.Tick(memCtrl)
		}

		subID := rec.taskID("sub-trans")
		require.NotZero(t, subID)

		var refreshMs []tasks.Milestone
		for _, m := range rec.milestonesOn(subID) {
			if m.Kind == tasks.MilestoneKindHardwareResource &&
				m.What == memCtrl.Name()+".refresh" {
				refreshMs = append(refreshMs, m)
			}
		}

		// The refresh stall is attributed to the sub-transaction whose command
		// resumes issuing once the window ends: at least one refresh milestone
		// (one per stall window the command waited through) is charged to it,
		// rather than the window being invisible in the trace.
		require.True(t, (len(refreshMs)) >= (1), "expected len(refreshMs) >= 1")
	})
}
