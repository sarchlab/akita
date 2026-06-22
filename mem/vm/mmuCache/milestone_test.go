package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// mmuCacheMilestoneRecorder captures the task starts, ends, and milestones the
// mmuCache emits, in emission order and without the DBTracer's same-key/same-
// time dedup, so a test can assert the full ordered set of blocking reasons and
// the task (buffer vs req_in/req_out) each one is attributed to.
type mmuCacheMilestoneRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	ends       []tracing.TaskEnd
	milestones []tracing.Milestone
}

func (r *mmuCacheMilestoneRecorder) StartTask(s tracing.TaskStart) {
	r.starts = append(r.starts, s)
}

func (r *mmuCacheMilestoneRecorder) EndTask(e tracing.TaskEnd) {
	r.ends = append(r.ends, e)
}

func (r *mmuCacheMilestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

// firstStart returns the first recorded task start of the given kind.
func (r *mmuCacheMilestoneRecorder) firstStart(kind string) (tracing.TaskStart, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tracing.TaskStart{}, false
}

func (r *mmuCacheMilestoneRecorder) hasEnd(id uint64) bool {
	for _, e := range r.ends {
		if e.ID == id {
			return true
		}
	}

	return false
}

func (r *mmuCacheMilestoneRecorder) milestonesOn(taskID uint64) []tracing.Milestone {
	var ms []tracing.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *mmuCacheMilestoneRecorder) kindsOn(taskID uint64) []tracing.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tracing.MilestoneKind, len(ms))
	for i, m := range ms {
		ks[i] = m.Kind
	}

	return ks
}

var _ = Describe("MMUCache milestones", func() {
	var (
		comp       *Comp
		mw         *mmuCacheMiddleware
		topPort    messaging.Port
		bottomPort messaging.Port
		rec        *mmuCacheMilestoneRecorder
	)

	BeforeEach(func() {
		engine := timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		spec := DefaultSpec()
		spec.NumBlocks = 4
		spec.NumLevels = 2
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{
				LowModulePort: messaging.RemotePort("LowModule"),
				UpModulePort:  messaging.RemotePort("UpModule"),
			}).
			Build("MMUCache")

		assignDefaultPorts(reg, comp)

		topPort = comp.GetPortByName("Top")
		bottomPort = comp.GetPortByName("Bottom")
		controlPort := comp.GetPortByName("Control")
		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(bottomPort)
		(&noopConn{}).PlugIn(controlPort)

		mw = &mmuCacheMiddleware{comp: comp}

		rec = &mmuCacheMilestoneRecorder{}
		tracing.CollectTrace(comp, rec)
		// The admission milestones land on the Top/Bottom incoming-buffer tasks,
		// so the test needs those buffer tasks to exist.
		tracing.CollectIncomingBufferTrace(topPort)
		tracing.CollectIncomingBufferTrace(bottomPort)
	})

	makeTopReq := func(vAddr uint64) vmprotocol.TranslationReq {
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("UpModule")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 3
		req.TrafficClass = "vmprotocol.TranslationReq"
		return req
	}

	// driveRoundTrip pushes a Top translation request all the way through the
	// mmuCache: deliver and forward the walk downstream, then deliver the
	// matching bottom response and forward the translation upstream. It returns
	// the Top request and the forwarded Bottom request ID.
	driveRoundTrip := func(vAddr uint64) (vmprotocol.TranslationReq, uint64) {
		req := makeTopReq(vAddr)
		topPort.Deliver(req)

		Expect(mw.lookup()).To(BeTrue())

		sent := bottomPort.RetrieveOutgoing().(vmprotocol.TranslationReq)
		bottomReqID := sent.ID

		page := vm.Page{
			PID:   req.PID,
			VAddr: vAddr,
			PAddr: 0x5000,
			Valid: true,
		}
		rsp := vmprotocol.TranslationRsp{Page: page}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = messaging.RemotePort("LowModule")
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = bottomReqID
		rsp.TrafficClass = "vmprotocol.TranslationRsp"
		bottomPort.Deliver(rsp)

		Expect(mw.handleBottomPort()).To(BeTrue())

		// Drain the upstream response so a follow-up send would not block.
		topPort.RetrieveOutgoing()

		return req, bottomReqID
	}

	It("opens and closes req_in for the Top request and req_out for the "+
		"forwarded walk", func() {
		req, bottomReqID := driveRoundTrip(0x2000)

		// req_in opens for the Top request and closes when the response returns.
		// Its ID is read from the recorded start because the completion path has
		// already forgotten the receiver-task registry entry by now.
		reqInStart, ok := rec.firstStart("req_in")
		Expect(ok).To(BeTrue())
		Expect(reqInStart.ID).ToNot(BeZero())
		Expect(reqInStart.ParentID).To(Equal(req.ID))
		Expect(rec.hasEnd(reqInStart.ID)).To(BeTrue())

		// req_out opens for the forwarded Bottom request, parented to req_in, and
		// closes when its response returns.
		reqOutStart, ok := rec.firstStart("req_out")
		Expect(ok).To(BeTrue())
		Expect(reqOutStart.ID).To(Equal(bottomReqID))
		Expect(reqOutStart.ParentID).To(Equal(reqInStart.ID))
		Expect(rec.hasEnd(bottomReqID)).To(BeTrue())

		// The completion path released the in-flight entry — no leak.
		Expect(comp.State.InflightReqs).To(BeEmpty())
	})

	// bufTaskID returns the ID of the first incoming-buffer task located at the
	// given port. Buffer task IDs are read from recorded starts because the
	// retrieve that admits the message has already forgotten their registry
	// entries by the time the test inspects them.
	bufTaskID := func(portName string) uint64 {
		for _, s := range rec.starts {
			if s.Kind == tracing.IncomingBufferTaskKind &&
				s.Location == portName+".incoming" {
				return s.ID
			}
		}
		return 0
	}

	It("attributes admission waits to the incoming-buffer tasks", func() {
		driveRoundTrip(0x2000)

		// The Top buffer task: reached the head of the Top buffer, then waited
		// for the Bottom port to be free to forward the walk.
		topBufID := bufTaskID(topPort.Name())
		Expect(topBufID).ToNot(BeZero())
		Expect(rec.kindsOn(topBufID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindQueue,
			tracing.MilestoneKindNetworkBusy,
		}))
		topBufMs := rec.milestonesOn(topBufID)
		Expect(topBufMs[0].What).To(Equal(topPort.Name()))
		Expect(topBufMs[1].What).To(Equal(bottomPort.Name()))

		// The Bottom buffer task: reached the head of the Bottom buffer, then
		// waited for the Top port to be free to forward the response upstream.
		bottomBufID := bufTaskID(bottomPort.Name())
		Expect(bottomBufID).ToNot(BeZero())
		Expect(rec.kindsOn(bottomBufID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindQueue,
			tracing.MilestoneKindNetworkBusy,
		}))
		bottomBufMs := rec.milestonesOn(bottomBufID)
		Expect(bottomBufMs[0].What).To(Equal(bottomPort.Name()))
		Expect(bottomBufMs[1].What).To(Equal(topPort.Name()))
	})

	It("charges the downstream walk-fetch wait to the req_in as a "+
		"translation milestone", func() {
		driveRoundTrip(0x2000)

		// req_in opens at retrieve and is completed when the downstream response
		// returns. Its ID is read from the recorded start because the completion
		// path has already forgotten the receiver-task registry entry by now.
		reqInStart, ok := rec.firstStart("req_in")
		Expect(ok).To(BeTrue())

		// The in-flight wait on the forwarded walk is charged to the original
		// req_in as a translation milestone when the response returns.
		reqInMs := rec.milestonesOn(reqInStart.ID)
		Expect(reqInMs).ToNot(BeEmpty())

		var translationMs *tracing.Milestone
		for i := range reqInMs {
			if reqInMs[i].Kind == tracing.MilestoneKindTranslation {
				translationMs = &reqInMs[i]
				break
			}
		}
		Expect(translationMs).ToNot(BeNil())
		Expect(translationMs.What).To(Equal("translation"))
	})

	It("ends the in-flight req_in and req_out on a mid-walk reset so no "+
		"tasks are left unended", func() {
		req := makeTopReq(0x2000)
		topPort.Deliver(req)
		Expect(mw.lookup()).To(BeTrue())
		sent := bottomPort.RetrieveOutgoing().(vmprotocol.TranslationReq)
		bottomReqID := sent.ID

		// The walk is in flight: req_in and req_out are open but not yet ended,
		// and the response has not returned.
		Expect(comp.State.InflightReqs).To(HaveLen(1))
		reqInStart, ok := rec.firstStart("req_in")
		Expect(ok).To(BeTrue())
		reqOutStart, ok := rec.firstStart("req_out")
		Expect(ok).To(BeTrue())
		Expect(reqOutStart.ID).To(Equal(bottomReqID))
		Expect(rec.hasEnd(reqInStart.ID)).To(BeFalse())
		Expect(rec.hasEnd(reqOutStart.ID)).To(BeFalse())

		// Reset while the walk is in flight.
		reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
		reset.ID = timing.GetIDGenerator().Generate()
		reset.Src = messaging.RemotePort("CtrlAgent")
		reset.Dst = comp.GetPortByName("Control").AsRemote()
		reset.TrafficClass = "memcontrolprotocol.Req"
		comp.GetPortByName("Control").Deliver(reset)

		acked := false
		for i := 0; i < 64 && !acked; i++ {
			comp.Tick()
			if out := comp.GetPortByName("Control").RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(memcontrolprotocol.Rsp); ok &&
					rsp.Command == memcontrolprotocol.CmdReset {
					acked = true
				}
			}
		}
		Expect(acked).To(BeTrue())

		// The reset ended both tasks (finalize req_out, complete req_in) instead
		// of merely forgetting the registry entry, so no task is left unended and
		// the in-flight table is empty.
		Expect(rec.hasEnd(reqInStart.ID)).To(BeTrue())
		Expect(rec.hasEnd(reqOutStart.ID)).To(BeTrue())
		Expect(comp.State.InflightReqs).To(BeEmpty())
	})
})
