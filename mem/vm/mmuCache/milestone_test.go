package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// mmuCacheMilestoneRecorder captures the task starts, ends, and milestones the
// mmuCache emits, in emission order and without the DBTracer's same-key/same-
// time dedup, so a test can assert the full ordered set of blocking reasons and
// the task (buffer vs req_in/req_out) each one is attributed to.
type mmuCacheMilestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	ends       []tasks.Task
	milestones []tasks.Milestone
}

func (r *mmuCacheMilestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *mmuCacheMilestoneRecorder) TaskEnded(e tasks.Task) {
	r.ends = append(r.ends, e)
}

func (r *mmuCacheMilestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

// firstStart returns the first recorded task start of the given kind.
func (r *mmuCacheMilestoneRecorder) firstStart(kind string) (tasks.Task, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tasks.Task{}, false
}

func (r *mmuCacheMilestoneRecorder) hasEnd(id uint64) bool {
	for _, e := range r.ends {
		if e.ID == id {
			return true
		}
	}

	return false
}

func (r *mmuCacheMilestoneRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *mmuCacheMilestoneRecorder) kindsOn(taskID uint64) []tasks.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tasks.MilestoneKind, len(ms))
	for i, m := range ms {
		ks[i] = m.Kind
	}

	return ks
}

// milestoneClock lets manual middleware ticks observe nonzero waits without
// running the event queue, which would also admit or retire test traffic.
type milestoneClock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

func (c *milestoneClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

func TestMMUCacheMilestones(t *testing.T) {
	var (
		engine     timing.Engine
		comp       *Comp
		mw         *mmuCacheMiddleware
		topPort    messaging.Port
		bottomPort messaging.Port
		rec        *mmuCacheMilestoneRecorder
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim := testsim.New(engine, true)

		spec := DefaultSpec()
		spec.NumBlocks = 4
		spec.NumLevels = 2
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{
				LowModulePort: messaging.RemotePort("LowModule"),
				UpModulePort:  messaging.RemotePort("UpModule"),
			}).
			Build("MMUCache")
		compPorts := defaultPorts(sim, "MMUCache")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		topPort = comp.Ports.Top
		bottomPort = comp.Ports.Bottom
		controlPort := comp.Ports.Control
		(&noopConn{}).BindPort(topPort)
		(&noopConn{}).BindPort(bottomPort)
		(&noopConn{}).BindPort(controlPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		mw = comp.Middlewares.Cache

		rec = &mmuCacheMilestoneRecorder{}
		comp.TaskTracker().Subscribe(rec, comp)
		// The admission milestones land on the Top/Bottom incoming-buffer tasks,
		// so the test needs those buffer tasks to exist.
	}

	makeTopReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 3},
			ID:  comp.NewID(),
			Src: messaging.RemotePort("UpModule"),
			Dst: topPort.AsRemote(),
		}

		return req
	}

	// driveRoundTrip pushes a Top translation request all the way through the
	// mmuCache: deliver and forward the walk downstream, then deliver the
	// matching bottom response and forward the translation upstream. It returns
	// the Top request and the forwarded Bottom request ID.
	driveRoundTrip := func(t *testing.T, vAddr uint64) (messaging.Msg, uint64) {
		t.Helper()

		req := makeTopReq(vAddr)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000

		require.True(t, mw.lookup())

		sentValue, _ := bottomPort.RetrieveOutgoing()

		sent := sentValue
		bottomReqID := sent.ID

		page := vm.Page{
			PID:   req.Payload.(addrtransprotocol.Req).PID,
			VAddr: vAddr,
			PAddr: 0x5000,
			Valid: true,
		}
		rsp := messaging.Msg{Payload: addrtransprotocol.Rsp{Page: page},
			ID:    comp.NewID(),
			Src:   messaging.RemotePort("LowModule"),
			Dst:   bottomPort.AsRemote(),
			RspTo: bottomReqID,
		}

		bottomPort.Deliver(rsp)

		engine.(*milestoneClock).now += 1000

		require.True(t, mw.handleBottomPort())

		// Drain the upstream response so a follow-up send would not block.
		topPort.RetrieveOutgoing()

		return req, bottomReqID
	}

	t.Run("opens and closes req_in for the Top request and req_out for the "+
		"forwarded walk", func(t *testing.T) {
		setup1(t)

		req, bottomReqID := driveRoundTrip(t, 0x2000)

		// req_in opens for the Top request and closes when the response returns.
		// Its ID is read from the start; the completed transaction was removed.
		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)
		require.NotZero(t, reqInStart.ID)
		require.Equal(t, req.ID, reqInStart.MsgID)
		require.Zero(t, reqInStart.ParentID)
		require.True(t, rec.hasEnd(reqInStart.ID))

		// req_out opens for the forwarded Bottom request, parented to req_in, and
		// closes when its response returns.
		reqOutStart, ok := rec.firstStart("req_out")
		require.True(t, ok)
		require.Equal(t, bottomReqID, reqOutStart.MsgID)
		require.NotEqual(t, bottomReqID, reqOutStart.ID)
		require.Equal(t, reqInStart.ID, reqOutStart.ParentID)
		require.True(t, rec.hasEnd(reqOutStart.ID))

		// The completion path released the in-flight entry — no leak.
		require.Empty(t, comp.State.InflightReqs)
	})

	// bufTaskID returns the ID of the first incoming-buffer task located at the
	// given port. Buffer task IDs are read from recorded starts because the
	// retrieve has already removed their port entries.
	bufTaskID := func(portName string) uint64 {
		for _, s := range rec.starts {
			if s.Kind == tasks.KindIncomingBuffer &&
				s.Location == portName+".incoming" {
				return s.ID
			}
		}
		return 0
	}

	t.Run("attributes admission waits to the incoming-buffer tasks", func(t *testing.T) {
		setup1(t)

		driveRoundTrip(t, 0x2000)

		// The Top buffer task has no queue delay and waited
		// for the Bottom port to be free to forward the walk.
		topBufID := bufTaskID(topPort.Name())
		require.NotZero(t, topBufID)
		require.Equal(t, []tasks.MilestoneKind{
			tasks.MilestoneKindNetworkBusy,
		}, rec.kindsOn(topBufID))
		topBufMs := rec.milestonesOn(topBufID)

		require.Equal(t, bottomPort.Name(), topBufMs[0].What)

		// The Bottom buffer task has no queue delay and
		// waited for the Top port to be free to forward the response upstream.
		bottomBufID := bufTaskID(bottomPort.Name())
		require.NotZero(t, bottomBufID)
		require.Equal(t, []tasks.MilestoneKind{
			tasks.MilestoneKindNetworkBusy,
		}, rec.kindsOn(bottomBufID))
		bottomBufMs := rec.milestonesOn(bottomBufID)

		require.Equal(t, topPort.Name(), bottomBufMs[0].What)
	})

	t.Run("charges the downstream walk-fetch wait to the req_in as a "+
		"translation milestone", func(t *testing.T) {
		setup1(t)

		driveRoundTrip(t, 0x2000)

		// req_in opens at retrieve and is completed when the downstream response
		// returns. Read its original identity from the recorded start.
		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)

		// The in-flight wait on the forwarded walk is charged to the original
		// req_in as a translation milestone when the response returns.
		reqInMs := rec.milestonesOn(reqInStart.ID)
		require.NotEmpty(t, reqInMs)

		var translationMs *tasks.Milestone
		for i := range reqInMs {
			if reqInMs[i].Kind == tasks.MilestoneKindTranslation {
				translationMs = &reqInMs[i]
				break
			}
		}
		require.NotNil(t, translationMs)
		require.Equal(t, "translation", translationMs.What)
	})

	t.Run("ends the in-flight req_in and req_out on a mid-walk reset so no "+
		"tasks are left unended", func(t *testing.T) {
		setup1(t)

		req := makeTopReq(0x2000)
		topPort.Deliver(req)
		engine.(*milestoneClock).now += 1000
		require.True(t, mw.lookup())
		sentValue, _ := bottomPort.RetrieveOutgoing()
		sent := sentValue
		bottomReqID := sent.ID

		// The walk is in flight: req_in and req_out are open but not yet ended,
		// and the response has not returned.
		require.Len(t, comp.State.InflightReqs, 1)
		reqInStart, ok := rec.firstStart("req_in")
		require.True(t, ok)
		reqOutStart, ok := rec.firstStart("req_out")
		require.True(t, ok)
		require.Equal(t, bottomReqID, reqOutStart.MsgID)
		require.NotEqual(t, bottomReqID, reqOutStart.ID)
		require.False(t, rec.hasEnd(reqInStart.ID))
		require.False(t, rec.hasEnd(reqOutStart.ID))

		// Reset while the walk is in flight.
		reset := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
			ID:  comp.NewID(),
			Src: messaging.RemotePort("CtrlAgent"),
			Dst: comp.Ports.Control.AsRemote(),
		}

		comp.Ports.Control.Deliver(reset)

		acked := false
		for i := 0; i < 64 && !acked; i++ {
			modelingtest.Tick(comp)
			if out, ok := comp.Ports.Control.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdReset {
					acked = true
				}
			}
		}
		require.True(t, acked)

		// Reset ends both original tasks before clearing the in-flight table.
		require.True(t, rec.hasEnd(reqInStart.ID))
		require.True(t, rec.hasEnd(reqOutStart.ID))
		require.Empty(t, comp.State.InflightReqs)
	})
}
