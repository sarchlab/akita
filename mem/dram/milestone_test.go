package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// milestoneRecorder captures the task starts and milestones the DRAM emits, in
// emission order and without the DBTracer's same-key/same-time dedup, so a test
// can assert the blocking reasons and the task (buffer vs sub-trans) each one is
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

var _ = Describe("DRAM admission milestones", func() {
	var (
		engine  timing.Engine
		memCtrl *modeling.Component[Spec, State, Resources]
		topPort messaging.Port
		rec     *milestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)
		memCtrl = MakeBuilder().
			WithRegistrar(reg).
			Build("MemCtrl")

		for _, name := range []string{"Top", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(memCtrl).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			memCtrl.AssignPort(name, p)
		}

		topPort = memCtrl.GetPortByName("Top")

		// Attach the recorder before driving so MsgIDAtReceiver and
		// MsgIDAtIncomingBuffer hand out real task IDs (they return 0 with no
		// hooks). The admission milestone lands on the Top-port buffer task, so
		// the buffer task must exist.
		rec = &milestoneRecorder{}
		tracing.CollectTrace(memCtrl, rec)
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

	It("records a SubTransQueue admission milestone on the buffer task", func() {
		topPort.Deliver(makeRead(0x40))

		// One tick runs parseTop, which admits the request: the buffer task's
		// queue admission milestone is emitted just before RetrieveIncoming.
		memCtrl.Tick()

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		Expect(bufID).ToNot(BeZero())

		bufMs := rec.milestonesOn(bufID)
		// The buffer task carries the reached-head milestone (from the port
		// hook) followed by the SubTransQueue admission milestone.
		Expect(bufMs).To(HaveLen(2))
		Expect(bufMs[0].Kind).To(Equal(tracing.MilestoneKindQueue))
		Expect(bufMs[0].What).To(Equal(topPort.Name()))
		Expect(bufMs[1].Kind).To(Equal(tracing.MilestoneKindQueue))
		Expect(bufMs[1].What).To(Equal(memCtrl.Name() + ".SubTransQueue"))
	})
})

var _ = Describe("DRAM refresh-stall attribution", func() {
	It("charges a refresh milestone to the sub-trans that issues "+
		"after a refresh window", func() {
		engine := timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		// A short tREFI/tRFC so a refresh window opens quickly and the request
		// that arrives during it is the one charged the stall.
		spec := DefaultSpec()
		spec.TREFI = 1
		spec.TRFC = 3

		memCtrl := MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("MemCtrl")

		for _, name := range []string{"Top", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(memCtrl).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			memCtrl.AssignPort(name, p)
		}

		topPort := memCtrl.GetPortByName("Top")

		rec := &milestoneRecorder{}
		tracing.CollectTrace(memCtrl, rec)
		tracing.CollectIncomingBufferTrace(topPort)

		read := memprotocol.ReadReq{Address: 0x40, AccessByteSize: 4}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = messaging.RemotePort("Agent")
		read.Dst = topPort.AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"
		topPort.Deliver(read)

		// Tick enough for the refresh window to open and close and the command
		// to finally issue.
		for i := 0; i < 20; i++ {
			memCtrl.Tick()
		}

		subID := rec.taskID("sub-trans")
		Expect(subID).ToNot(BeZero())

		var refreshMs []tracing.Milestone
		for _, m := range rec.milestonesOn(subID) {
			if m.Kind == tracing.MilestoneKindHardwareResource &&
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
