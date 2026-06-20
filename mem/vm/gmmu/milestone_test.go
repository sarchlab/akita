package gmmu

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

// milestoneRecorder captures the task starts and milestones the GMMU emits, in
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

var _ = Describe("GMMU milestones", func() {
	const (
		agentPort     = messaging.RemotePort("Agent")
		lowModulePort = messaging.RemotePort("LowModule")
	)

	var (
		engine     timing.Engine
		pageTable  vm.PageTable
		gmmuComp   *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		walk       *walkMW
		respond    *respondMW
		rec        *milestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		pageTable = vm.NewPageTable(12)

		spec := DefaultSpec()
		spec.DeviceID = 0
		spec.Latency = 1
		spec.LowModule = lowModulePort

		reg := modeling.NewStandaloneRegistrar(engine)
		gmmuComp = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{PageTable: pageTable}).
			WithSpec(spec).
			Build("GMMU")

		assignDefaultPorts(reg, gmmuComp)

		topPort = gmmuComp.GetPortByName("Top")
		bottomPort = gmmuComp.GetPortByName("Bottom")

		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(bottomPort)
		(&noopConn{}).PlugIn(gmmuComp.GetPortByName("Control"))

		walk = gmmuComp.Middlewares()[1].(*walkMW)
		respond = gmmuComp.Middlewares()[2].(*respondMW)

		// Attach the recorder before driving so MsgIDAtIncomingBuffer hands out
		// real task IDs (it returns 0 when there are no hooks).
		rec = &milestoneRecorder{}
		tracing.CollectTrace(gmmuComp, rec)
		// The Top-port admission milestone lands on the Top-port buffer task and
		// the Bottom-port admission milestone on the Bottom-port buffer task, so
		// the test needs both buffer tasks to exist.
		tracing.CollectIncomingBufferTrace(topPort)
		tracing.CollectIncomingBufferTrace(bottomPort)
	})

	makeReq := func(vAddr uint64) vmprotocol.TranslationReq {
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = agentPort
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 0
		req.TrafficClass = "vmprotocol.TranslationReq"
		return req
	}

	It("emits a hardware_resource admission milestone on the Top buffer task "+
		"when it admits the head request", func() {
		req := makeReq(0x1000)
		topPort.Deliver(req)

		Expect(walk.parseFromTop()).To(BeTrue())

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
		Expect(bufMs[1].What).To(Equal(gmmuComp.Name() + ".walk"))
	})

	It("does not admit (and emits no admission milestone) while servicing "+
		"the max in-flight requests", func() {
		gmmuComp.State.WalkingTranslations = make(
			[]transactionState, gmmuComp.Spec().MaxRequestsInFlight)

		req := makeReq(0x1000)
		topPort.Deliver(req)

		Expect(walk.parseFromTop()).To(BeFalse())

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		Expect(bufID).ToNot(BeZero())
		bufMs := rec.milestonesOn(bufID)
		Expect(bufMs).To(HaveLen(1))
		Expect(bufMs[0].Kind).To(Equal(tracing.MilestoneKindQueue))
	})

	It("emits a network_busy admission milestone on the Bottom buffer task "+
		"when it admits the head response", func() {
		// A remote walk is outstanding so the bottom response matches a
		// transaction and is forwarded upstream.
		walking := transactionState{
			ReqID:    timing.GetIDGenerator().Generate(),
			ReqSrc:   agentPort,
			ReqDst:   topPort.AsRemote(),
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1,
		}
		remoteReqID := timing.GetIDGenerator().Generate()
		gmmuComp.State.RemoteMemReqs = map[uint64]transactionState{
			remoteReqID: walking,
		}

		rsp := vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x2000},
		}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = lowModulePort
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = remoteReqID
		rsp.TrafficClass = "vmprotocol.TranslationRsp"
		bottomPort.Deliver(rsp)

		Expect(respond.fetchFromBottom()).To(BeTrue())

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		Expect(bufID).ToNot(BeZero())

		// The Bottom buffer task owns the at-head admission wait: reached the
		// head of the Bottom buffer, then waited for the busy Top port to free.
		bufMs := rec.milestonesOn(bufID)
		Expect(bufMs).To(HaveLen(2))
		Expect(bufMs[0].Kind).To(Equal(tracing.MilestoneKindQueue))
		Expect(bufMs[0].What).To(Equal(bottomPort.Name()))
		Expect(bufMs[1].Kind).To(Equal(tracing.MilestoneKindNetworkBusy))
		Expect(bufMs[1].What).To(Equal(topPort.Name()))
	})
})
