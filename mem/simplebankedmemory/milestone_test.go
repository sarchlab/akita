package simplebankedmemory

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// milestoneRecorder captures the task starts and milestones the memory emits, in
// emission order and without the DBTracer's same-key/same-time dedup, so a test
// can assert the blocking reason and the task (buffer vs req_in) it is
// attributed to.
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

var _ = Describe("SimpleBankedMemory admission milestones", func() {
	var (
		engine  timing.Engine
		memComp *Comp
		topPort messaging.Port
		rec     *milestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage := mem.NewStorage(4 * mem.GB)

		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 2

		reg := modeling.NewStandaloneRegistrar(engine)
		memComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{Storage: storage}).
			Build("Mem")

		assignPort(reg, memComp, "Top", 4)
		assignPort(reg, memComp, "Control", 16)

		topPort = memComp.GetPortByName("Top")

		// Attach the recorder before driving so MsgIDAtIncomingBuffer hands out a
		// real buffer task ID (it returns 0 with no hooks). The admission
		// milestone lands on the Top-port buffer task, so the buffer task must
		// exist.
		rec = &milestoneRecorder{}
		tracing.CollectTrace(memComp, rec)
		tracing.CollectIncomingBufferTrace(topPort)
	})

	makeRead := func(addr uint64) memprotocol.ReadReq {
		req := memprotocol.ReadReq{Address: addr, AccessByteSize: 4}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.TrafficBytes = 12
		req.TrafficClass = "memprotocol.ReadReq"
		return req
	}

	It("records a per-bank hardware_resource admission milestone on "+
		"the buffer task", func() {
		req := makeRead(0x0)
		topPort.Deliver(req)

		spec := memComp.Spec()
		bankID := selectBank(spec, bankSelectionAddress(spec, req.Address))

		// One tick runs dispatchFromTopPort, which admits the request: the
		// buffer task's bank admission milestone is emitted just before
		// RetrieveIncoming.
		memComp.Tick()

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		Expect(bufID).ToNot(BeZero())

		bufMs := rec.milestonesOn(bufID)
		// The buffer task carries the reached-head milestone (from the port
		// hook) followed by the bank admission milestone.
		Expect(bufMs).To(HaveLen(2))
		Expect(bufMs[0].Kind).To(Equal(tracing.MilestoneKindQueue))
		Expect(bufMs[0].What).To(Equal(topPort.Name()))
		Expect(bufMs[1].Kind).To(Equal(tracing.MilestoneKindHardwareResource))
		Expect(bufMs[1].What).To(Equal(
			fmt.Sprintf("%s.bank%d", memComp.Name(), bankID)))
	})
})
