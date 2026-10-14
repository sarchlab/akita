package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/internal/testsim"

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

var _ = Describe("DRAM admission milestones", func() {
	var (
		engine  timing.Engine
		sim     timing.Simulation
		memCtrl *Comp
		topPort messaging.Port
		rec     *milestoneRecorder
	)

	BeforeEach(func() {
		engine = &milestoneClock{Engine: timing.NewSerialEngine()}
		sim = testsim.New(engine, true)

		memCtrl = buildDRAM(sim, Definition.DefaultSpec, "MemCtrl", 16)
		Expect(sim.Initialize()).To(Succeed())

		topPort = memCtrl.Ports.Top

		// Subscribe before driving to retain the starts of request and buffer tasks.
		rec = &milestoneRecorder{}
		memCtrl.TaskTracker().Subscribe(rec, memCtrl)

	})

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Agent"),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	It("records a SubTransQueue admission milestone on the buffer task", func() {
		topPort.Deliver(makeRead(0x40))
		engine.(*milestoneClock).now += 1000

		// One tick runs parseTop, which admits the request: the buffer task's
		// queue admission milestone is emitted just before RetrieveIncoming.
		modelingtest.Tick(memCtrl)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		Expect(bufID).ToNot(BeZero())

		bufMs := rec.milestonesOn(bufID)
		// No queue delay: the buffer task carries only the SubTransQueue admission milestone.
		Expect(bufMs).To(HaveLen(1))

		Expect(bufMs[0].Kind).To(Equal(tasks.MilestoneKindQueue))
		Expect(bufMs[0].What).To(Equal(memCtrl.Name() + ".SubTransQueue"))
	})
})

var _ = Describe("DRAM refresh-stall attribution", func() {
	It("charges a refresh milestone to the sub-trans that issues "+
		"after a refresh window", func() {
		engine := &milestoneClock{Engine: timing.NewSerialEngine()}
		sim := testsim.New(engine, true)

		// A short tREFI/tRFC so a refresh window opens quickly and the request
		// that arrives during it is the one charged the stall.
		spec := Definition.DefaultSpec
		spec.TREFI = 1
		spec.TRFC = 3

		memCtrl := buildDRAM(sim, spec, "MemCtrl", 16)
		Expect(sim.Initialize()).To(Succeed())

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
		Expect(subID).ToNot(BeZero())

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
		Expect(len(refreshMs)).To(BeNumerically(">=", 1))
	})
})
