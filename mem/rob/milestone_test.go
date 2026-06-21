package rob

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// milestoneRecorder captures the task starts, milestones, and tags the ROB
// emits, in emission order and without the DBTracer's same-key/same-time dedup,
// so a test can assert the full ordered set of blocking reasons and the task
// (buffer vs req_in) each one is attributed to.
type milestoneRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	milestones []tracing.Milestone
	tags       []tracing.TaskTag
}

func (r *milestoneRecorder) StartTask(s tracing.TaskStart) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *milestoneRecorder) AddTaskTag(t tracing.TaskTag) {
	r.tags = append(r.tags, t)
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

func (r *milestoneRecorder) kindsOn(taskID uint64) []tracing.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tracing.MilestoneKind, len(ms))
	for i, m := range ms {
		ks[i] = m.Kind
	}

	return ks
}

var _ = Describe("Reorder Buffer milestones", func() {
	const (
		topRemote        = messaging.RemotePort("Agent")
		bottomUnitRemote = messaging.RemotePort("BottomUnit")
	)

	var (
		rob        *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		rec        *milestoneRecorder
	)

	BeforeEach(func() {
		engine := timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		spec := DefaultSpec()
		spec.BufferSize = 4
		spec.NumReqPerCycle = 2
		spec.BottomUnit = bottomUnitRemote

		rob = MakeBuilder().WithRegistrar(reg).WithSpec(spec).Build("Rob")

		assign := func(name string, bufSize int) messaging.Port {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(rob).
				WithSpec(modeling.PortSpec{BufSize: bufSize}).
				Build(name)
			rob.AssignPort(name, p)
			return p
		}

		topPort = assign("Top", 4)
		bottomPort = assign("Bottom", 4)
		ctrlPort := assign("Control", 2)

		for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}

		rec = &milestoneRecorder{}
		tracing.CollectTrace(rob, rec)
		// The admission milestones now land on the Top-port buffer task, so the
		// test needs the buffer task to exist.
		tracing.CollectIncomingBufferTrace(topPort)
	})

	makeRead := func(addr uint64) memprotocol.ReadReq {
		req := memprotocol.ReadReq{Address: addr, AccessByteSize: 4}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = topRemote
		req.Dst = topPort.AsRemote()
		req.TrafficClass = "memprotocol.ReadReq"
		return req
	}

	makeWrite := func(addr uint64, data []byte) memprotocol.WriteReq {
		req := memprotocol.WriteReq{Address: addr, Data: data}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = topRemote
		req.Dst = topPort.AsRemote()
		req.TrafficClass = "memprotocol.WriteReq"
		return req
	}

	// driveRoundTrip pushes a request all the way through the ROB: admit and
	// forward downstream (tick 1), parse the bottom response (tick 2, after the
	// in-tick bottomUp-before-parseBottom ordering exposes it the next tick),
	// then retire and respond upstream (tick 3).
	driveRoundTrip := func(req memprotocol.AccessReq, rsp messaging.Msg) {
		topPort.Deliver(req)

		rob.Tick()
		shadowID := rob.State.Transactions[0].ReqToBottomID
		bottomPort.RetrieveOutgoing()

		switch r := rsp.(type) {
		case memprotocol.DataReadyRsp:
			r.RspTo = shadowID
			bottomPort.Deliver(r)
		case memprotocol.WriteDoneRsp:
			r.RspTo = shadowID
			bottomPort.Deliver(r)
		}

		rob.Tick() // parseBottom records the response
		rob.Tick() // bottomUp retires the head and responds
	}

	It("records admission milestones on the buffer task and processing "+
		"milestones on req_in, plus a read tag, for a read", func() {
		rsp := memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = bottomUnitRemote
		rsp.Dst = bottomPort.AsRemote()
		rsp.TrafficClass = "memprotocol.DataReadyRsp"

		driveRoundTrip(makeRead(0), rsp)

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		reqInID := rec.taskID("req_in")
		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(bufID).ToNot(Equal(reqInID))

		// The buffer task owns the pre-admission story: reached the head of the
		// Top buffer, then waited for a free reorder slot, then to send the shadow.
		Expect(rec.kindsOn(bufID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindQueue,
			tracing.MilestoneKindHardwareResource,
			tracing.MilestoneKindNetworkBusy,
		}))
		bufMs := rec.milestonesOn(bufID)
		Expect(bufMs[0].What).To(Equal(topPort.Name()))
		Expect(bufMs[1].What).To(Equal(rob.Name() + ".buffer"))
		Expect(bufMs[2].What).To(Equal(bottomPort.Name()))

		// req_in owns only the post-admission processing.
		Expect(rec.kindsOn(reqInID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindData,
			tracing.MilestoneKindDependency,
			tracing.MilestoneKindNetworkBusy,
		}))
		reqMs := rec.milestonesOn(reqInID)
		Expect(reqMs[0].What).To(Equal(bottomPort.Name()))
		Expect(reqMs[1].What).To(Equal(rob.Name() + ".reorder"))
		Expect(reqMs[2].What).To(Equal(topPort.Name()))

		Expect(rec.tags).To(HaveLen(1))
		Expect(rec.tags[0].What).To(Equal("read"))
		Expect(rec.tags[0].TaskID).To(Equal(reqInID))
	})

	It("distinguishes a write with a subtask milestone and a write tag", func() {
		rsp := memprotocol.WriteDoneRsp{}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = bottomUnitRemote
		rsp.Dst = bottomPort.AsRemote()
		rsp.TrafficClass = "memprotocol.WriteDoneRsp"

		driveRoundTrip(makeWrite(64, []byte{1, 2, 3, 4}), rsp)

		reqInID := rec.taskID("req_in")
		Expect(rec.kindsOn(reqInID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindSubTask,
			tracing.MilestoneKindDependency,
			tracing.MilestoneKindNetworkBusy,
		}))

		Expect(rec.tags).To(HaveLen(1))
		Expect(rec.tags[0].What).To(Equal("write"))
	})

	It("emits the dependency milestone before the response-sent milestone "+
		"so the in-order-commit reason wins a same-tick tie", func() {
		rsp := memprotocol.DataReadyRsp{Data: []byte{0xAB}}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = bottomUnitRemote
		rsp.Dst = bottomPort.AsRemote()
		rsp.TrafficClass = "memprotocol.DataReadyRsp"

		driveRoundTrip(makeRead(0), rsp)

		var depIdx, topSendIdx int
		for i, m := range rec.milestones {
			switch {
			case m.Kind == tracing.MilestoneKindDependency:
				depIdx = i
			case m.Kind == tracing.MilestoneKindNetworkBusy &&
				m.What == topPort.Name():
				topSendIdx = i
			}
		}
		Expect(depIdx).To(BeNumerically("<", topSendIdx))
	})
})
