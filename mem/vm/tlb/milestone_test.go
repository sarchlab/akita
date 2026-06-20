package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// tlbMilestoneRecorder captures the task starts, ends, milestones, and tags the
// TLB emits, in emission order and without the DBTracer's dedup, so a test can
// assert the buffer-vs-req_in attribution of each event and the parent/child
// relationship between req_in and the pipeline subtask.
type tlbMilestoneRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	ends       []tracing.TaskEnd
	milestones []tracing.Milestone
	tags       []tracing.TaskTag
}

func (r *tlbMilestoneRecorder) StartTask(s tracing.TaskStart) {
	r.starts = append(r.starts, s)
}

func (r *tlbMilestoneRecorder) EndTask(e tracing.TaskEnd) {
	r.ends = append(r.ends, e)
}

func (r *tlbMilestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *tlbMilestoneRecorder) AddTaskTag(t tracing.TaskTag) {
	r.tags = append(r.tags, t)
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
func (r *tlbMilestoneRecorder) taskStart(kind string) (tracing.TaskStart, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tracing.TaskStart{}, false
}

func (r *tlbMilestoneRecorder) milestonesOn(taskID uint64) []tracing.Milestone {
	var ms []tracing.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *tlbMilestoneRecorder) kindsOn(taskID uint64) []tracing.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tracing.MilestoneKind, len(ms))
	for i, m := range ms {
		ks[i] = m.Kind
	}

	return ks
}

func (r *tlbMilestoneRecorder) hasMilestone(
	taskID uint64, kind tracing.MilestoneKind, what string,
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
	for _, t := range r.tags {
		if t.TaskID == taskID {
			ws = append(ws, t.What)
		}
	}

	return ws
}

var _ = Describe("TLB milestones", func() {
	const remotePort = messaging.RemotePort("RemotePort")

	var (
		engine  timing.Engine
		tlbComp *Comp
		topPort messaging.Port
		rec     *tlbMilestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.NumSets = 1
		spec.NumWays = 32
		spec.Log2PageSize = 12

		reg := modeling.NewStandaloneRegistrar(engine)
		tlbComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: remotePort,
				},
			}).
			Build("TLB")

		assignDefaultPorts(reg, tlbComp)
		plugNoopConn(tlbComp)

		topPort = tlbComp.GetPortByName("Top")

		rec = &tlbMilestoneRecorder{}
		tracing.CollectTrace(tlbComp, rec)
		// The admission milestone lands on the Top-port buffer task, so the test
		// needs the buffer task to exist.
		tracing.CollectIncomingBufferTrace(topPort)
	})

	makeReq := func(vAddr uint64) vmprotocol.TranslationReq {
		req := vmprotocol.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 1
		req.TrafficClass = "vmprotocol.TranslationReq"
		return req
	}

	// drive ticks the component until the request has passed through the pipeline
	// and been looked up, or until a generous tick budget is exhausted.
	drive := func(maxTicks int) {
		for i := 0; i < maxTicks; i++ {
			tlbComp.Tick()
		}
	}

	It("attributes the at-head wait to the buffer task and opens a pipeline "+
		"subtask under req_in for a hit", func() {
		// Pre-load the page so the lookup hits.
		next := &tlbComp.State
		page := vm.Page{PID: 1, VAddr: 0x100, PAddr: 0x200, Valid: true}
		setUpdate(&next.Sets[0], 1, page)
		setVisit(&next.Sets[0], 1)

		req := makeReq(0x100)
		topPort.Deliver(req)

		drive(20)

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		reqInID := rec.taskID("req_in")
		pipeStart, hasPipe := rec.taskStart(tracing.PipelineTaskKind)

		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(bufID).ToNot(Equal(reqInID))
		Expect(hasPipe).To(BeTrue())

		// (a) The buffer task carries the hardware_resource admission milestone
		// for the at-head wait on a free pipeline slot.
		Expect(rec.hasMilestone(bufID, tracing.MilestoneKindHardwareResource,
			tlbComp.Name()+".pipeline")).To(BeTrue())

		// (b) The pipeline subtask is parented to req_in.
		Expect(pipeStart.ParentID).To(Equal(reqInID))
		Expect(pipeStart.What).To(Equal(tlbComp.Name() + ".pipeline"))
		// It is closed once the request leaves the pipeline at lookup.
		var pipeEnded bool
		for _, e := range rec.ends {
			if e.ID == pipeStart.ID {
				pipeEnded = true
			}
		}
		Expect(pipeEnded).To(BeTrue())

		// (c) The post-lookup data milestone and the hit tag attach to req_in.
		Expect(rec.hasMilestone(reqInID, tracing.MilestoneKindData,
			tlbComp.Name()+".Sets")).To(BeTrue())
		Expect(rec.hasMilestone(reqInID, tracing.MilestoneKindNetworkBusy,
			topPort.Name())).To(BeTrue())
		Expect(rec.tagsOn(reqInID)).To(ContainElement("hit"))

		// The admission milestone is NOT on req_in (it belongs to the buffer task).
		Expect(rec.kindsOn(reqInID)).ToNot(
			ContainElement(tracing.MilestoneKindHardwareResource))
	})

	It("attributes the at-head wait to the buffer task and keeps the MSHR and "+
		"network milestones on req_in for a miss", func() {
		// Page present but invalid -> miss path that fetches from the bottom.
		next := &tlbComp.State
		page := vm.Page{PID: 1, VAddr: 0x100, PAddr: 0x200, Valid: false}
		setUpdate(&next.Sets[0], 1, page)
		setVisit(&next.Sets[0], 1)

		req := makeReq(0x100)
		topPort.Deliver(req)

		drive(20)

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		reqInID := rec.taskID("req_in")
		pipeStart, hasPipe := rec.taskStart(tracing.PipelineTaskKind)

		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(hasPipe).To(BeTrue())

		// (a) Admission milestone on the buffer task.
		Expect(rec.hasMilestone(bufID, tracing.MilestoneKindHardwareResource,
			tlbComp.Name()+".pipeline")).To(BeTrue())

		// (b) Pipeline subtask parented to req_in.
		Expect(pipeStart.ParentID).To(Equal(reqInID))

		// (c) The post-lookup MSHR hardware_resource and the bottom-port
		// network_busy milestones now attach to the already-open req_in, and the
		// miss tag is on req_in.
		Expect(rec.hasMilestone(reqInID, tracing.MilestoneKindHardwareResource,
			tlbComp.Name()+".MSHR")).To(BeTrue())
		bottomPort := tlbComp.GetPortByName("Bottom")
		Expect(rec.hasMilestone(reqInID, tracing.MilestoneKindNetworkBusy,
			bottomPort.Name())).To(BeTrue())
		Expect(rec.tagsOn(reqInID)).To(ContainElement("miss"))
	})
})
