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

// taskStart returns the first recorded task start of the given kind and whether
// one was found.
func (r *milestoneRecorder) taskStart(kind string) (tracing.TaskStart, bool) {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s, true
		}
	}

	return tracing.TaskStart{}, false
}

// ended reports whether a task with the given ID was ended.
func (r *milestoneRecorder) ended(taskID uint64) bool {
	for _, e := range r.ends {
		if e.ID == taskID {
			return true
		}
	}

	return false
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

var _ = Describe("SimpleBankedMemory pipeline-traversal milestones", func() {
	var (
		engine  timing.Engine
		storage *mem.Storage
		memComp *Comp
		topPort messaging.Port
		agent   *testAgent
		conn    *loopbackConnection
		rec     *milestoneRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(4 * mem.GB)

		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 3

		reg := modeling.NewStandaloneRegistrar(engine)
		memComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{Storage: storage}).
			Build("Mem")

		assignPort(reg, memComp, "Top", 4)
		assignPort(reg, memComp, "Control", 16)

		topPort = memComp.GetPortByName("Top")
		agent = newTestAgent("Agent")
		conn = newLoopbackConnection("Conn")
		conn.PlugIn(topPort)
		conn.PlugIn(agent.port)

		// Attach the recorder before driving so the receiver-side task IDs are
		// real (MsgIDAtReceiver returns 0 with no hooks).
		rec = &milestoneRecorder{}
		tracing.CollectTrace(memComp, rec)
	})

	// drive ticks the memory and shuttles messages over the loopback until the
	// agent has received the response, or a tick budget is exhausted.
	drive := func() {
		for i := 0; i < 64 && len(agent.received) == 0; i++ {
			memComp.Tick()
			conn.transfer()
		}
	}

	It("attributes the bank-pipeline traversal as work on the read "+
		"req_in", func() {
		data := []byte{1, 2, 3, 4}
		Expect(storage.Write(0x40, data)).To(Succeed())

		read := memprotocol.ReadReq{Address: 0x40, AccessByteSize: 4}
		read.ID = timing.GetIDGenerator().Generate()
		read.Src = agent.port.AsRemote()
		read.Dst = topPort.AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"

		agent.send(read)
		conn.transfer()
		drive()

		Expect(agent.received).To(HaveLen(1))
		rsp, ok := agent.received[0].(memprotocol.DataReadyRsp)
		Expect(ok).To(BeTrue())
		Expect(rsp.Data).To(Equal(data))

		reqInID := rec.taskID("req_in")
		Expect(reqInID).ToNot(BeZero())

		// The req_in carries a work milestone marking the end of the
		// bank-pipeline traversal, named "<comp>.pipeline".
		reqInMs := rec.milestonesOn(reqInID)
		Expect(reqInMs).To(ContainElement(SatisfyAll(
			HaveField("Kind", tracing.MilestoneKindWork),
			HaveField("What", memComp.Name()+".pipeline"),
		)))

		// The pipeline subtask is a child of req_in and is closed at exit.
		pipeStart, found := rec.taskStart(tracing.PipelineTaskKind)
		Expect(found).To(BeTrue())
		Expect(pipeStart.ParentID).To(Equal(reqInID))
		Expect(pipeStart.What).To(Equal(memComp.Name() + ".pipeline"))
		Expect(rec.ended(pipeStart.ID)).To(BeTrue())
	})

	It("attributes the bank-pipeline traversal as work on the write "+
		"req_in", func() {
		write := memprotocol.WriteReq{
			Address: 0x80,
			Data:    []byte{9, 8, 7, 6},
		}
		write.ID = timing.GetIDGenerator().Generate()
		write.Src = agent.port.AsRemote()
		write.Dst = topPort.AsRemote()
		write.TrafficBytes = len(write.Data) + 12
		write.TrafficClass = "memprotocol.WriteReq"

		agent.send(write)
		conn.transfer()
		drive()

		Expect(agent.received).To(HaveLen(1))
		_, ok := agent.received[0].(memprotocol.WriteDoneRsp)
		Expect(ok).To(BeTrue())

		reqInID := rec.taskID("req_in")
		Expect(reqInID).ToNot(BeZero())

		reqInMs := rec.milestonesOn(reqInID)
		Expect(reqInMs).To(ContainElement(SatisfyAll(
			HaveField("Kind", tracing.MilestoneKindWork),
			HaveField("What", memComp.Name()+".pipeline"),
		)))

		pipeStart, found := rec.taskStart(tracing.PipelineTaskKind)
		Expect(found).To(BeTrue())
		Expect(pipeStart.ParentID).To(Equal(reqInID))
		Expect(rec.ended(pipeStart.ID)).To(BeTrue())
	})
})
