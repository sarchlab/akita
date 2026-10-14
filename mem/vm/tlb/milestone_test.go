package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/internal/testsim"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
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

func (r *tlbMilestoneRecorder) TagAdded(t tasks.Tag) {
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
	for _, t := range r.tags {
		if t.TaskID == taskID {
			ws = append(ws, t.What)
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

var _ = Describe("TLB milestones", func() {
	const remotePort = messaging.RemotePort("RemotePort")

	var (
		engine  timing.Engine
		sim     timing.Simulation
		tlbComp *Comp
		topPort messaging.Port
		rec     *tlbMilestoneRecorder
	)

	BeforeEach(func() {
		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)

		spec := Definition.DefaultSpec
		spec.NumSets = 1
		spec.NumWays = 32
		spec.Log2PageSize = 12

		spec.TranslationMapperType = "single"
		spec.TranslationProviders = []messaging.RemotePort{remotePort}
		tlbComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
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

	})

	makeReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		return req
	}

	// drive ticks the component until the request has passed through the pipeline
	// and been looked up, or until a generous tick budget is exhausted.
	drive := func(maxTicks int) {
		for i := 0; i < maxTicks; i++ {
			modelingtest.Tick(tlbComp)
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
		engine.(*milestoneClock).now += 1000

		drive(20)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		pipeStart, hasPipe := rec.taskStart(tasks.KindPipeline)

		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(bufID).ToNot(Equal(reqInID))
		Expect(hasPipe).To(BeTrue())

		// (a) The buffer task carries the hardware_resource admission milestone
		// for the at-head wait on a free pipeline slot.
		Expect(rec.hasMilestone(bufID, tasks.MilestoneKindHardwareResource,
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

		// The pipeline traversal is recorded as work on req_in — not a blocking
		// reason and not the data/MSHR milestone.
		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindWork,
			tlbComp.Name()+".pipeline")).To(BeTrue())

		// (c) The post-lookup data milestone and the hit tag attach to req_in.
		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindData,
			tlbComp.Name()+".Sets")).To(BeTrue())
		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindNetworkBusy,
			topPort.Name())).To(BeTrue())
		Expect(rec.tagsOn(reqInID)).To(ContainElement("hit"))

		// The admission milestone is NOT on req_in (it belongs to the buffer task).
		Expect(rec.kindsOn(reqInID)).ToNot(
			ContainElement(tasks.MilestoneKindHardwareResource))
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
		engine.(*milestoneClock).now += 1000

		drive(20)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		pipeStart, hasPipe := rec.taskStart(tasks.KindPipeline)

		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(hasPipe).To(BeTrue())

		// (a) Admission milestone on the buffer task.
		Expect(rec.hasMilestone(bufID, tasks.MilestoneKindHardwareResource,
			tlbComp.Name()+".pipeline")).To(BeTrue())

		// (b) Pipeline subtask parented to req_in.
		Expect(pipeStart.ParentID).To(Equal(reqInID))

		// The pipeline traversal is recorded as work on req_in, not the MSHR.
		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindWork,
			tlbComp.Name()+".pipeline")).To(BeTrue())

		// (c) The post-lookup MSHR hardware_resource and the bottom-port
		// network_busy milestones attach to the already-open req_in; the miss tag
		// is on req_in.
		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindHardwareResource,
			tlbComp.Name()+".MSHR")).To(BeTrue())
		bottomPort := tlbComp.Ports.Bottom
		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindNetworkBusy,
			bottomPort.Name())).To(BeTrue())
		Expect(rec.tagsOn(reqInID)).To(ContainElement("miss"))

		// (d) Drive the downstream fetch response back. The translation wait (the
		// time the req_out was in flight) is recorded as a translation milestone
		// on req_in, and the response-send network_busy on the Top port comes
		// only afterwards — not during the req_out.
		fetchValue, _ := bottomPort.RetrieveOutgoing()
		fetch := fetchValue
		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x100, PAddr: 0x200, Valid: true},
		},
			ID:           sim.NewID(),
			Src:          remotePort,
			Dst:          bottomPort.AsRemote(),
			RspTo:        fetch.ID,
			TrafficClass: "vmprotocol.TranslationRsp"}

		bottomPort.Deliver(rsp)

		engine.(*milestoneClock).now += 1000
		drive(20)

		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindTranslation,
			"translation")).To(BeTrue())
		Expect(rec.hasMilestone(reqInID, tasks.MilestoneKindNetworkBusy,
			topPort.Name())).To(BeTrue())

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
		Expect(transIdx).To(BeNumerically(">=", 0))
		Expect(topIdx).To(BeNumerically(">=", 0))
		Expect(transIdx).To(BeNumerically("<", topIdx))
	})
})
