package gmmu

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

// milestoneRecorder captures the task starts and milestones the GMMU emits, in
// emission order and without the DBTracer's dedup, so a test can assert which
// task (incoming-buffer vs req_in) each admission milestone is attributed to.
type milestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	ends       []tasks.Task
	milestones []tasks.Milestone
}

func (r *milestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) TaskEnded(e tasks.Task) {
	r.ends = append(r.ends, e)
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

func (r *milestoneRecorder) hasMilestone(
	taskID uint64, kind tasks.MilestoneKind, what string,
) bool {
	for _, m := range r.milestonesOn(taskID) {
		if m.Kind == kind && m.What == what {
			return true
		}
	}

	return false
}

// taskStart returns the first recorded task start of the given kind.
func (r *milestoneRecorder) taskStart(kind string) (tasks.Task, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tasks.Task{}, false
}

// milestoneClock lets manual middleware ticks observe nonzero waits without
// running the event queue, which would also admit or retire test traffic.
type milestoneClock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

func (c *milestoneClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestGMMUMilestones(t *testing.T) {
	const (
		agentPort     = messaging.RemotePort("Agent")
		lowModulePort = messaging.RemotePort("LowModule")
	)

	var (
		engine     timing.Engine
		sim        timing.Simulation
		pageTable  *vm.PageTable
		gmmuComp   *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		walk       *walkMW
		respond    *respondMW
		rec        *milestoneRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)
		pageTable = vm.NewPageTable(12)

		spec := DefaultSpec()
		spec.DeviceID = 0
		spec.Latency = 1
		spec.Bottom = lowModulePort

		gmmuComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			Build("GMMU")
		gmmuCompPorts := defaultPorts("GMMU")
		gmmuComp.BindPort("Top", gmmuCompPorts.Top)
		gmmuComp.BindPort("Bottom", gmmuCompPorts.Bottom)
		gmmuComp.BindPort("Control", gmmuCompPorts.Control)

		topPort = gmmuComp.Ports.Top
		bottomPort = gmmuComp.Ports.Bottom

		(&noopConn{}).BindPort(topPort)
		(&noopConn{}).BindPort(bottomPort)
		(&noopConn{}).BindPort(gmmuComp.Ports.Control)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		walk = gmmuComp.Middlewares.Walk
		respond = gmmuComp.Middlewares.Respond

		// Attach the recorder before driving so buffer enqueue allocates
		// real task IDs (it returns 0 when there are no hooks).
		rec = &milestoneRecorder{}
		gmmuComp.TaskTracker().Subscribe(rec, gmmuComp)
		// The Top-port admission milestone lands on the Top-port buffer task and
		// the Bottom-port admission milestone on the Bottom-port buffer task, so
		// the test needs both buffer tasks to exist.
	}

	makeReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 0},
			ID:  sim.NewID(),
			Src: agentPort,
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		return req
	}

	t.Run("emits a hardware_resource admission milestone on the Top buffer task "+
		"when it admits the head request", func(t *testing.T) {
		setup1(t)

		req := makeReq(0x1000)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		require.True(t, walk.parseFromTop())

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
		require.Equal(t, gmmuComp.Name()+".walk", bufMs[0].What)
	})

	t.Run("does not admit (and emits no admission milestone) while servicing "+
		"the max in-flight requests", func(t *testing.T) {
		setup1(t)

		gmmuComp.State.WalkingTranslations = make(
			[]transactionState, gmmuComp.Spec.MaxRequestsInFlight)

		req := makeReq(0x1000)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		require.False(t, walk.parseFromTop())

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		require.NotZero(t, bufID)
		bufMs := rec.milestonesOn(bufID)
		require.Empty(t, bufMs)
	})

	t.Run("emits a network_busy admission milestone on the Bottom buffer task "+
		"when it admits the head response", func(t *testing.T) {
		setup1(t)

		// A remote walk is outstanding so the bottom response matches a
		// transaction and is forwarded upstream.
		walking := transactionState{
			ReqID:    sim.NewID(),
			ReqSrc:   agentPort,
			ReqDst:   topPort.AsRemote(),
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1,
		}
		remoteReqID := sim.NewID()
		gmmuComp.State.RemoteMemReqs = map[uint64]transactionState{
			remoteReqID: walking,
		}

		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x2000},
		},
			ID:           sim.NewID(),
			Src:          lowModulePort,
			Dst:          bottomPort.AsRemote(),
			RspTo:        remoteReqID,
			TrafficClass: "vmprotocol.TranslationRsp"}

		bottomPort.Deliver(rsp)

		engine.(*milestoneClock).now += 1000

		require.True(t, respond.fetchFromBottom())

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		require.NotZero(t, bufID)

		// The Bottom buffer task owns the at-head admission wait: reached the
		// head of the Bottom buffer, then waited for the busy Top port to free.
		bufMs := rec.milestonesOn(bufID)
		require.Len(t, bufMs, 1)

		require.Equal(t, tasks.MilestoneKindNetworkBusy, bufMs[0].Kind)
		require.Equal(t, topPort.Name(), bufMs[0].What)
	})

	t.Run("records the local page-walk latency as a work milestone on req_in "+
		"for a local hit", func(t *testing.T) {
		setup1(t)

		// Page resident on this device -> the fast local-walk path.
		page := vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x2000,
			DeviceID: 0,
			Valid:    true,
		}
		pageTable.Insert(page)

		topPort.Deliver(makeReq(0x1000))

		engine.(*milestoneClock).now += 1000

		// Tick 1: parseFromTop admits the request (opens req_in).
		// Tick 2: walkPageTable decrements CycleLeft (latency=1 -> 0).
		// Tick 3: CycleLeft==0, local hit completes and responds upstream.
		modelingtest.Tick(gmmuComp)
		modelingtest.Tick(gmmuComp)
		modelingtest.Tick(gmmuComp)

		reqInID := rec.taskID("req_in")
		require.NotZero(t, reqInID)

		// The local walk's processing latency is recorded as work on req_in.
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindWork,
			gmmuComp.Name()+".walk"))
	})

	t.Run("opens the remote req_out under req_in and records the in-flight wait "+
		"as a translation milestone on req_in for a remote walk", func(t *testing.T) {
		setup1(t)

		// Page resident on a different device -> the remote-walk path: the GMMU
		// sends a downstream req_out on Bottom and waits for the response.
		page := vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x2000,
			DeviceID: 1,
			Valid:    true,
		}
		pageTable.Insert(page)

		topPort.Deliver(makeReq(0x1000))

		engine.(*milestoneClock).now += 1000

		// Tick 1: parseFromTop admits the request (opens req_in).
		// Tick 2: walkPageTable decrements CycleLeft (latency=1 -> 0).
		// Tick 3: CycleLeft==0, page is remote, sends downstream req_out.
		modelingtest.Tick(gmmuComp)
		modelingtest.Tick(gmmuComp)
		modelingtest.Tick(gmmuComp)

		reqI, _ := bottomPort.RetrieveOutgoing()
		require.NotNil(t, reqI)
		downstream := reqI

		reqInID := rec.taskID("req_in")
		require.NotZero(t, reqInID)

		// (a) The downstream req_out is a child task of req_in.
		reqOut, hasReqOut := rec.taskStart("req_out")
		require.True(t, hasReqOut)
		require.Equal(t, downstream.ID, reqOut.MsgID)
		require.NotEqual(t, downstream.ID, reqOut.ID)
		require.Equal(t, reqInID, reqOut.ParentID)

		// Deliver the remote response.
		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{Page: page},
			ID:           sim.NewID(),
			Src:          lowModulePort,
			Dst:          bottomPort.AsRemote(),
			RspTo:        downstream.ID,
			TrafficClass: "vmprotocol.TranslationRsp"}

		bottomPort.Deliver(rsp)

		engine.(*milestoneClock).now += 1000

		// Tick: fetchFromBottom forwards upstream and finalizes the walk.
		modelingtest.Tick(gmmuComp)

		rspToTopI, _ := topPort.RetrieveOutgoing()
		require.NotNil(t, rspToTopI)

		// (b) The remote-walk in-flight wait is recorded as a translation
		// milestone on the ORIGINAL req_in.
		require.True(t, rec.hasMilestone(reqInID, tasks.MilestoneKindTranslation,
			"translation"))

		// (c) The downstream req_out subtask is finalized when the response
		// returns.
		var reqOutEnded bool
		for _, e := range rec.ends {
			if e.ID == reqOut.ID {
				reqOutEnded = true
			}
		}
		require.True(t, reqOutEnded)
	})
}
