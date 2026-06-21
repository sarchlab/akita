package mmu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// milestoneRecorder captures the task starts and milestones the MMU emits, in
// emission order and without the DBTracer's dedup, so a test can assert which
// task (incoming-buffer vs req_in) each admission milestone is attributed to.
type milestoneRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	milestones []tracing.Milestone
}

func (r *milestoneRecorder) StartTask(s tracing.TaskStart) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) AddMilestone(m tracing.Milestone) {
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

func (r *milestoneRecorder) milestonesOn(taskID uint64) []tracing.Milestone {
	var ms []tracing.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

var _ = Describe("MMU milestones", func() {
	var (
		engine    timing.Engine
		pageTable vm.PageTable
		mmuComp   *Comp
		topPort   messaging.Port
		mw        *translationMW
		rec       *milestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		pageTable = vm.NewPageTable(12)

		reg := modeling.NewStandaloneRegistrar(engine)
		mmuComp = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(DefaultSpec()).
			Build("MMU")

		topPort = assignPort(reg, mmuComp, "Top", 16)
		assignPort(reg, mmuComp, "Control", 4)

		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(mmuComp.GetPortByName("Control"))

		mw = mmuComp.Middlewares()[1].(*translationMW)

		// Attach the recorder before driving so MsgIDAtIncomingBuffer hands out
		// real task IDs (it returns 0 when there are no hooks).
		rec = &milestoneRecorder{}
		tracing.CollectTrace(mmuComp, rec)
		// The admission milestone lands on the Top-port buffer task, so the test
		// needs the buffer task to exist.
		tracing.CollectIncomingBufferTrace(topPort)
	})

	makeReq := func(vAddr uint64) vmprotocol.TranslationReq {
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent.Top")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 0
		req.TrafficClass = "vmprotocol.TranslationReq"
		return req
	}

	It("emits a hardware_resource admission milestone on the buffer task "+
		"when it admits the head request", func() {
		req := makeReq(0x1000)
		topPort.Deliver(req)

		Expect(mw.parseFromTop()).To(BeTrue())

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		reqInID := rec.taskID("req_in")
		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(bufID).ToNot(Equal(reqInID))

		// The buffer task owns the at-head admission wait: reached the head of
		// the Top buffer, then waited for a free walk slot.
		bufMs := rec.milestonesOn(bufID)
		Expect(bufMs).To(HaveLen(2))
		Expect(bufMs[0].Kind).To(Equal(tracing.MilestoneKindQueue))
		Expect(bufMs[0].What).To(Equal(topPort.Name()))
		Expect(bufMs[1].Kind).To(Equal(tracing.MilestoneKindHardwareResource))
		Expect(bufMs[1].What).To(Equal(mmuComp.Name() + ".walk"))
	})

	It("labels the local page-walk latency as work on the req_in when the "+
		"walk completes", func() {
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

		// Admit the request (opens req_in and starts the walk), then drive the
		// CycleLeft countdown to completion so doPageWalkHit sends the response
		// and emits the walk milestone.
		Expect(mw.parseFromTop()).To(BeTrue())

		sent := false
		for i := 0; i < 64 && !sent; i++ {
			mw.Tick()
			if out := topPort.RetrieveOutgoing(); out != nil {
				if _, ok := out.(vmprotocol.TranslationRsp); ok {
					sent = true
				}
			}
		}
		Expect(sent).To(BeTrue())

		// The walk milestone lands on the req_in (walking.RecvTaskID ==
		// MsgIDAtReceiver(req)), labelling the local-walk latency as work.
		reqInID := rec.taskID("req_in")
		Expect(reqInID).ToNot(BeZero())

		reqInMs := rec.milestonesOn(reqInID)
		var workMs *tracing.Milestone
		for i := range reqInMs {
			if reqInMs[i].Kind == tracing.MilestoneKindWork {
				workMs = &reqInMs[i]
				break
			}
		}
		Expect(workMs).ToNot(BeNil())
		Expect(workMs.What).To(Equal(mmuComp.Name() + ".walk"))
	})

	It("does not admit (and emits no admission milestone) while servicing "+
		"the max in-flight requests", func() {
		mmuComp.State = State{
			WalkingTranslations: make([]transactionState, 16),
		}

		req := makeReq(0x1000)
		topPort.Deliver(req)

		Expect(mw.parseFromTop()).To(BeFalse())

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		Expect(bufID).ToNot(BeZero())
		// Only the reached-head queue milestone; no admission milestone since the
		// request was never admitted.
		bufMs := rec.milestonesOn(bufID)
		Expect(bufMs).To(HaveLen(1))
		Expect(bufMs[0].Kind).To(Equal(tracing.MilestoneKindQueue))
	})
})
