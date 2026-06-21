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
	ends       []tracing.TaskEnd
	milestones []tracing.Milestone
}

func (r *milestoneRecorder) StartTask(s tracing.TaskStart) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) EndTask(e tracing.TaskEnd) {
	r.ends = append(r.ends, e)
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

func (r *milestoneRecorder) hasMilestone(
	taskID uint64, kind tracing.MilestoneKind, what string,
) bool {
	for _, m := range r.milestonesOn(taskID) {
		if m.Kind == kind && m.What == what {
			return true
		}
	}

	return false
}

// taskStart returns the first recorded task start of the given kind.
func (r *milestoneRecorder) taskStart(kind string) (tracing.TaskStart, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tracing.TaskStart{}, false
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

	It("records the local page-walk latency as a work milestone on req_in "+
		"for a local hit", func() {
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

		// Tick 1: parseFromTop admits the request (opens req_in).
		// Tick 2: walkPageTable decrements CycleLeft (latency=1 -> 0).
		// Tick 3: CycleLeft==0, local hit completes and responds upstream.
		gmmuComp.Tick()
		gmmuComp.Tick()
		gmmuComp.Tick()

		reqInID := rec.taskID("req_in")
		Expect(reqInID).ToNot(BeZero())

		// The local walk's processing latency is recorded as work on req_in.
		Expect(rec.hasMilestone(reqInID, tracing.MilestoneKindWork,
			gmmuComp.Name()+".walk")).To(BeTrue())
	})

	It("opens the remote req_out under req_in and records the in-flight wait "+
		"as a translation milestone on req_in for a remote walk", func() {
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

		// Tick 1: parseFromTop admits the request (opens req_in).
		// Tick 2: walkPageTable decrements CycleLeft (latency=1 -> 0).
		// Tick 3: CycleLeft==0, page is remote, sends downstream req_out.
		gmmuComp.Tick()
		gmmuComp.Tick()
		gmmuComp.Tick()

		reqI := bottomPort.RetrieveOutgoing()
		Expect(reqI).ToNot(BeNil())
		downstream := reqI.(vmprotocol.TranslationReq)

		reqInID := rec.taskID("req_in")
		Expect(reqInID).ToNot(BeZero())

		// (a) The downstream req_out is a child task of req_in.
		reqOut, hasReqOut := rec.taskStart("req_out")
		Expect(hasReqOut).To(BeTrue())
		Expect(reqOut.ID).To(Equal(downstream.ID))
		Expect(reqOut.ParentID).To(Equal(reqInID))

		// Deliver the remote response.
		rsp := vmprotocol.TranslationRsp{Page: page}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = lowModulePort
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = downstream.ID
		rsp.TrafficClass = "vmprotocol.TranslationRsp"
		bottomPort.Deliver(rsp)

		// Tick: fetchFromBottom forwards upstream and finalizes the walk.
		gmmuComp.Tick()

		rspToTopI := topPort.RetrieveOutgoing()
		Expect(rspToTopI).ToNot(BeNil())

		// (b) The remote-walk in-flight wait is recorded as a translation
		// milestone on the ORIGINAL req_in.
		Expect(rec.hasMilestone(reqInID, tracing.MilestoneKindTranslation,
			"translation")).To(BeTrue())

		// (c) The downstream req_out subtask is finalized when the response
		// returns.
		var reqOutEnded bool
		for _, e := range rec.ends {
			if e.ID == downstream.ID {
				reqOutEnded = true
			}
		}
		Expect(reqOutEnded).To(BeTrue())
	})
})
