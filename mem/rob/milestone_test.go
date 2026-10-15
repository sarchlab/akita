package rob

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/tasks"
)

// milestoneRecorder captures the task starts, milestones, and tags the ROB
// emits, in emission order and without the DBTracer's same-key/same-time dedup,
// so a test can assert the full ordered set of blocking reasons and the task
// (buffer vs req_in) each one is attributed to.
type milestoneRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	milestones []tasks.Milestone
	tags       []tasks.Tag
}

func (r *milestoneRecorder) TaskStarted(s tasks.Task) {
	r.starts = append(r.starts, s)
}

func (r *milestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *milestoneRecorder) TagAdded(t tasks.Tag) {
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

func (r *milestoneRecorder) milestonesOn(taskID uint64) []tasks.Milestone {
	var ms []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *milestoneRecorder) kindsOn(taskID uint64) []tasks.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tasks.MilestoneKind, len(ms))
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
		sim := akitasim.MakeBuilder().WithTaskTracking().Build()

		spec := Definition.DefaultSpec
		spec.BufferSize = 4
		spec.NumReqPerCycle = 2
		spec.Bottom = bottomUnitRemote

		port := func(name string, bufSize int) messaging.Port {
			return twowaybuffered.NewPort(bufSize, bufSize)
		}

		topPort = port("Top", 4)
		bottomPort = port("Bottom", 4)
		ctrlPort := port("Control", 2)

		rob = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			Build("Rob")

		rob.BindPort("Top", topPort)
		rob.BindPort("Bottom", bottomPort)
		rob.BindPort("Control", ctrlPort)

		for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
			conn := &noopConn{}
			conn.BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		rec = &milestoneRecorder{}
		rob.TaskTracker().Subscribe(rec, rob)
		// The admission milestones now land on the Top-port buffer task, so the
		// test needs the buffer task to exist.

	})

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: 4},
			ID:           rob.NewID(),
			Src:          topRemote,
			Dst:          topPort.AsRemote(),
			TrafficClass: "memprotocol.ReadReq"}

		return req
	}

	makeWrite := func(addr uint64, data []byte) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.WriteReq{Address: addr, Data: data},
			ID:           rob.NewID(),
			Src:          topRemote,
			Dst:          topPort.AsRemote(),
			TrafficClass: "memprotocol.WriteReq"}

		return req
	}

	// driveRoundTrip pushes a request all the way through the ROB: admit and
	// forward downstream (tick 1), parse the bottom response (tick 2, after the
	// in-tick bottomUp-before-parseBottom ordering exposes it the next tick),
	// then retire and respond upstream (tick 3).
	driveRoundTrip := func(req messaging.Msg, rsp messaging.Msg) {
		topPort.Deliver(req)

		modelingtest.Tick(rob)
		shadowID := rob.State.Transactions[0].ReqToBottomID
		bottomPort.RetrieveOutgoing()

		switch rsp.Payload.(type) {
		case memprotocol.DataReadyRsp:
			r := rsp

			r.RspTo = shadowID
			bottomPort.Deliver(r)
		case memprotocol.WriteDoneRsp:
			r := rsp

			r.RspTo = shadowID
			bottomPort.Deliver(r)
		}

		modelingtest.Tick(rob) // parseBottom records the response
		modelingtest.Tick(rob) // bottomUp retires the head and responds
	}

	It("records admission milestones on the buffer task and processing "+
		"milestones on req_in, plus a read tag, for a read", func() {
		rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}},
			ID:           rob.NewID(),
			Src:          bottomUnitRemote,
			Dst:          bottomPort.AsRemote(),
			TrafficClass: "memprotocol.DataReadyRsp"}

		driveRoundTrip(makeRead(0), rsp)

		bufID := rec.taskID(tasks.KindIncomingBuffer)
		reqInID := rec.taskID("req_in")
		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(bufID).ToNot(Equal(reqInID))

		// This manual round trip stays at one simulation timestamp, so all
		// buffer waits have zero duration and are suppressed.
		Expect(rec.milestonesOn(bufID)).To(BeEmpty())

		// req_in owns only the post-admission processing.
		Expect(rec.kindsOn(reqInID)).To(Equal([]tasks.MilestoneKind{
			tasks.MilestoneKindData,
			tasks.MilestoneKindDependency,
			tasks.MilestoneKindNetworkBusy,
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
		rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
			ID:           rob.NewID(),
			Src:          bottomUnitRemote,
			Dst:          bottomPort.AsRemote(),
			TrafficClass: "memprotocol.WriteDoneRsp"}

		driveRoundTrip(makeWrite(64, []byte{1, 2, 3, 4}), rsp)

		reqInID := rec.taskID("req_in")
		Expect(rec.kindsOn(reqInID)).To(Equal([]tasks.MilestoneKind{
			tasks.MilestoneKindSubTask,
			tasks.MilestoneKindDependency,
			tasks.MilestoneKindNetworkBusy,
		}))

		Expect(rec.tags).To(HaveLen(1))
		Expect(rec.tags[0].What).To(Equal("write"))
	})

	It("emits the dependency milestone before the response-sent milestone "+
		"so the in-order-commit reason wins a same-tick tie", func() {
		rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{Data: []byte{0xAB}},
			ID:           rob.NewID(),
			Src:          bottomUnitRemote,
			Dst:          bottomPort.AsRemote(),
			TrafficClass: "memprotocol.DataReadyRsp"}

		driveRoundTrip(makeRead(0), rsp)

		var depIdx, topSendIdx int
		for i, m := range rec.milestones {
			switch {
			case m.Kind == tasks.MilestoneKindDependency:
				depIdx = i
			case m.Kind == tasks.MilestoneKindNetworkBusy &&
				m.What == topPort.Name():
				topSendIdx = i
			}
		}
		Expect(depIdx).To(BeNumerically("<", topSendIdx))
	})
})
