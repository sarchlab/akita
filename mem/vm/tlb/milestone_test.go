package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// tlbMilestoneRecorder captures the task starts, ends, milestones, and tags the
// TLB emits, in emission order and without the DBTracer's dedup, so a test can
// assert the buffer-vs-req_in attribution of each event and the parent/child
// relationship between req_in and the pipeline subtask.
type tlbMilestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	ends       []tasks.Task
	milestones []tasks.Milestone
	tags       []tasks.Tag
}

func (r *tlbMilestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *tlbMilestoneRecorder) TaskEnded(e tasks.Task) {
	r.ends = append(r.ends, e)
}

func (r *tlbMilestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *tlbMilestoneRecorder) TagAdded(subject tasks.Tag) {
	r.tags = append(r.tags, subject)
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *tlbMilestoneRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

// taskStart returns the first recorded task start of the given kind.
func (r *tlbMilestoneRecorder) taskStart(kind string) (tasks.Task, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tasks.Task{}, false
}

func (r *tlbMilestoneRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *tlbMilestoneRecorder) kindsOn(taskID uint64) []tasks.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tasks.MilestoneKind, len(ms))
	for i, m := range ms {
		ks[i] = m.Kind
	}

	return ks
}

func (r *tlbMilestoneRecorder) hasMilestone(
	taskID uint64, kind tasks.MilestoneKind, what string,
) bool {
	for _, m := range r.milestonesOn(taskID) {
		if m.Kind == kind && m.What == what {
			return true
		}
	}

	return false
}

func (r *tlbMilestoneRecorder) tagsOn(taskID uint64) []string {
	var ws []string
	for _, subject := range r.tags {
		if subject.TaskID == taskID {
			ws = append(ws, subject.What)
		}
	}

	return ws
}

// milestoneClock lets manual middleware ticks observe nonzero waits without
// running the event queue, which would also admit or retire test traffic.
type milestoneClock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

func (c *milestoneClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

func TestTLBMilestones(t *testing.T) {
	const remotePort = messaging.RemotePort("RemotePort")

	var (
		engine  timing.Engine
		sim     timing.Simulation
		tlbComp *Comp
		topPort messaging.Port
		rec     *tlbMilestoneRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)

		spec := DefaultSpec()
		spec.NumSets = 1
		spec.NumWays = 32
		spec.Log2PageSize = 12

		spec.Bottom.Destinations = []messaging.RemotePort{remotePort}
		tlbComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("TLB")
		tlbCompPorts := defaultPorts("TLB")
		tlbComp.BindPort("Top", tlbCompPorts.Top)
		tlbComp.BindPort("Bottom", tlbCompPorts.Bottom)
		tlbComp.BindPort("Control", tlbCompPorts.Control)
		plugNoopConn(tlbComp)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		topPort = tlbComp.Ports.Top

		rec = &tlbMilestoneRecorder{}
		tlbComp.TaskTracker().Subscribe(rec, tlbComp)
		// The admission milestone lands on the Top-port buffer task, so the test
		// needs the buffer task to exist.
	}

	makeReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),
		}

		return req
	}

	// drive ticks the component until the request has passed through the pipeline
	// and been looked up, or until a generous tick budget is exhausted.
	drive := func(maxTicks int) {
		for i := 0; i < maxTicks; i++ {
			modelingtest.Tick(tlbComp)
		}
	}

	t.Run("attributes the at-head wait to the buffer task and opens a pipeline "+
		"subtask under req_in for a hit", func(t *testing.T) {
		setup1(t)

		// Pre-load the page so the lookup hits.
		next := &tlbComp.State
		page := vm.Page{PID: 1, VAddr: 0x100, PAddr: 0x200, Valid: true}
		setUpdate(&next.Sets[0], 1, page)
		setVisit(&next.Sets[0], 1)

		req := makeReq(0x100)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		drive(20)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		pipeStart, hasPipe := rec.taskStart(tasks.KindPipeline)

		require.NotZero(t, bufID)
		require.NotZero(t, reqInID)
		require.NotEqual(t, reqInID, bufID)
		require.True(t, hasPipe)

		// (a) The buffer task carries the hardware_resource admission milestone
		// for the at-head wait on a free pipeline slot.
		require.True(t, rec.hasMilestone(bufID, tasks.MilestoneKindHardwareResource,
			tlbComp.Name()+".pipeline"))

		// (b) The pipeline subtask is parented to req_in.
		require.Equal(t, reqInID, pipeStart.ParentID)
		require.Equal(t, tlbComp.Name()+".pipeline", pipeStart.What)
		// It is closed once the request leaves the pipeline at lookup.
		var pipeEnded bool
		for _, e := range rec.ends {
			if e.ID == pipeStart.ID {
				pipeEnded = true
			}
		}
		require.True(t, pipeEnded)

		// The pipeline traversal is recorded as work on req_in — not a blocking
		// reason and not the data/MSHR milestone.
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindWork,
			tlbComp.Name()+".pipeline"))

		// (c) The post-lookup data milestone and the hit tag attach to req_in.
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindData,
			tlbComp.Name()+".Sets"))
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindNetworkBusy,
			topPort.Name()))
		require.Contains(t, rec.tagsOn(reqInID), "hit")

		// The admission milestone is NOT on req_in (it belongs to the buffer task).
		require.NotContains(t, rec.kindsOn(reqInID), tasks.MilestoneKindHardwareResource)
	})

	t.Run("attributes the at-head wait to the buffer task and keeps the MSHR and "+
		"network milestones on req_in for a miss", func(t *testing.T) {
		setup1(t)

		// Page present but invalid -> miss path that fetches from the bottom.
		next := &tlbComp.State
		page := vm.Page{PID: 1, VAddr: 0x100, PAddr: 0x200, Valid: false}
		setUpdate(&next.Sets[0], 1, page)
		setVisit(&next.Sets[0], 1)

		req := makeReq(0x100)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		drive(20)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		pipeStart, hasPipe := rec.taskStart(tasks.KindPipeline)

		require.NotZero(t, bufID)
		require.NotZero(t, reqInID)
		require.True(t, hasPipe)

		// (a) Admission milestone on the buffer task.
		require.True(t, rec.hasMilestone(bufID, tasks.MilestoneKindHardwareResource,
			tlbComp.Name()+".pipeline"))

		// (b) Pipeline subtask parented to req_in.
		require.Equal(t, reqInID, pipeStart.ParentID)

		// The pipeline traversal is recorded as work on req_in, not the MSHR.
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindWork,
			tlbComp.Name()+".pipeline"))

		// (c) The post-lookup MSHR hardware_resource and the bottom-port
		// network_busy milestones attach to the already-open req_in; the miss tag
		// is on req_in.
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindHardwareResource,
			tlbComp.Name()+".MSHR"))
		bottomPort := tlbComp.Ports.Bottom
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindNetworkBusy,
			bottomPort.Name()))
		require.Contains(t, rec.tagsOn(reqInID), "miss")

		// (d) Drive the downstream fetch response back. The translation wait (the
		// time the req_out was in flight) is recorded as a translation milestone
		// on req_in, and the response-send network_busy on the Top port comes
		// only afterwards — not during the req_out.
		fetchValue, _ := bottomPort.RetrieveOutgoing()
		fetch := fetchValue
		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x100, PAddr: 0x200, Valid: true},
		},
			ID:    sim.NewID(),
			Src:   remotePort,
			Dst:   bottomPort.AsRemote(),
			RspTo: fetch.ID,
		}

		bottomPort.Deliver(rsp)

		engine.(*milestoneClock).now += 1000
		drive(20)

		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindTranslation,
			"translation"))
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindNetworkBusy,
			topPort.Name()))

		reqMs := rec.milestonesOn(reqInID)
		transIdx, topIdx := -1, -1
		for i, ms := range reqMs {
			switch {
			case ms.Kind == tasks.MilestoneKindTranslation:
				transIdx = i
			case ms.Kind == tasks.MilestoneKindNetworkBusy &&
				ms.What == topPort.Name():
				topIdx = i
			}
		}
		require.GreaterOrEqual(t, transIdx, 0)
		require.GreaterOrEqual(t, topIdx, 0)
		require.Less(t, transIdx, topIdx)
	})
}
