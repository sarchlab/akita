package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// dmTraceRecorder captures the full trace event stream the data mover emits —
// task starts/ends and milestones — in emission order and without the
// DBTracer's dedup, so a test can assert the admission milestone on the buffer
// task and that the req_in opened on the request is also ended.
type dmTraceRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	ends       []tracing.TaskEnd
	milestones []tracing.Milestone
}

func (r *dmTraceRecorder) StartTask(t tracing.TaskStart) {
	r.starts = append(r.starts, t)
}

func (r *dmTraceRecorder) EndTask(t tracing.TaskEnd) {
	r.ends = append(r.ends, t)
}

func (r *dmTraceRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *dmTraceRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

func (r *dmTraceRecorder) endedIDs() []uint64 {
	ids := make([]uint64, len(r.ends))
	for i, e := range r.ends {
		ids[i] = e.ID
	}

	return ids
}

func (r *dmTraceRecorder) milestonesOn(taskID uint64) []tracing.Milestone {
	var ms []tracing.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

var _ = Describe("DataMover milestones", func() {
	var (
		engine         timing.Engine
		dataMover      *modeling.Component[Spec, State, modeling.None]
		insideMem      *idealmemcontroller.Comp
		insideStorage  *mem.Storage
		outsideMem     *idealmemcontroller.Comp
		outsideStorage *mem.Storage
		conn           *directconnection.Comp
		srcPort        messaging.Port
		topPort        messaging.Port
		rec            *dmTraceRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		srcPort = messaging.NewPort(nil, 4, 4, "Src.Top")

		memSpec := idealmemcontroller.DefaultSpec()
		memSpec.Latency = 100
		memSpec.Width = 1
		memSpec.CacheLineSize = 64

		insideStorage = mem.NewStorage(1 * mem.MB)
		insideMem = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(memSpec).
			WithResources(idealmemcontroller.Resources{Storage: insideStorage}).
			Build("InsideMem")
		insideMem.AssignPort("Top",
			messaging.NewPort(insideMem, 16, 16, insideMem.Name()+".Top"))
		insideMem.AssignPort("Control",
			messaging.NewPort(insideMem, 16, 16, insideMem.Name()+".Control"))

		outsideStorage = mem.NewStorage(1 * mem.MB)
		outsideMem = idealmemcontroller.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(memSpec).
			WithResources(idealmemcontroller.Resources{Storage: outsideStorage}).
			Build("OutsideMem")
		outsideMem.AssignPort("Top",
			messaging.NewPort(outsideMem, 16, 16, outsideMem.Name()+".Top"))
		outsideMem.AssignPort("Control",
			messaging.NewPort(outsideMem, 16, 16, outsideMem.Name()+".Control"))

		dmSpec := DefaultSpec()
		dmSpec.BufferSize = 2048
		dmSpec.InsideByteGranularity = 64
		dmSpec.OutsideByteGranularity = 256

		dmReg := modeling.NewStandaloneRegistrar(engine)
		dataMover = MakeBuilder().
			WithRegistrar(dmReg).
			WithSpec(dmSpec).
			WithResources(Resources{
				InsideMapper: &mem.SinglePortMapper{
					Port: insideMem.GetPortByName("Top").AsRemote(),
				},
				OutsideMapper: &mem.SinglePortMapper{
					Port: outsideMem.GetPortByName("Top").AsRemote(),
				},
			}).
			Build("DataMover")

		assignDM := func(name string, bufSize int) {
			p := modeling.MakePortBuilder().
				WithRegistrar(dmReg).
				WithComponent(dataMover).
				WithSpec(modeling.PortSpec{BufSize: bufSize}).
				Build(name)
			dataMover.AssignPort(name, p)
		}
		assignDM("Top", 16)
		assignDM("Inside", 64)
		assignDM("Outside", 64)
		assignDM("Control", 40960000)

		topPort = dataMover.GetPortByName("Top")

		conn = directconnection.MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			Build("Conn")
		conn.PlugIn(srcPort)
		conn.PlugIn(topPort)
		conn.PlugIn(dataMover.GetPortByName("Inside"))
		conn.PlugIn(dataMover.GetPortByName("Outside"))
		conn.PlugIn(insideMem.GetPortByName("Top"))
		conn.PlugIn(outsideMem.GetPortByName("Top"))

		// Attach the recorder before driving so MsgIDAtReceiver and
		// MsgIDAtIncomingBuffer hand out real task IDs (they return 0 when there
		// are no hooks). The admission milestone lands on the Top-port buffer
		// task, so the test also needs that buffer task to exist.
		rec = &dmTraceRecorder{}
		tracing.CollectTrace(dataMover, rec)
		tracing.CollectIncomingBufferTrace(topPort)
	})

	It("attributes the admission wait to the buffer task and closes the "+
		"req_in that was opened on the request", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = topPort.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 0
		req.DstSide = "inside"
		req.ByteSize = 4096
		req.TrafficClass = "datamoverprotocol.DataMoveRequest"

		topPort.Deliver(req)

		engine.Run()

		// The move completed end-to-end.
		Expect(insideStorage.Read(0, 4096)).To(Equal(data))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))

		// (a) The admission milestone lands on the buffer task, not on req_in.
		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		reqInID := rec.taskID("req_in")
		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(bufID).ToNot(Equal(reqInID))

		bufMs := rec.milestonesOn(bufID)
		var admission *tracing.Milestone
		for i := range bufMs {
			if bufMs[i].Kind == tracing.MilestoneKindHardwareResource {
				admission = &bufMs[i]
			}
		}
		Expect(admission).ToNot(BeNil())
		Expect(admission.What).To(Equal(dataMover.Name() + ".transaction"))

		// (b) The req_in opened on the request is also ENDED. Before the
		// close-key fix it leaked because it was "closed" on the fresh response
		// (a different key), so no EndTask carried the req_in task id.
		Expect(rec.endedIDs()).To(ContainElement(reqInID))
	})

	It("charges the in-flight src-read and dst-write waits to the req_in "+
		"as data and subtask milestones", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := datamoverprotocol.DataMoveRequest{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = srcPort.AsRemote()
		req.Dst = topPort.AsRemote()
		req.SrcAddress = 0
		req.SrcSide = "outside"
		req.DstAddress = 0
		req.DstSide = "inside"
		req.ByteSize = 4096
		req.TrafficClass = "datamoverprotocol.DataMoveRequest"

		topPort.Deliver(req)

		engine.Run()

		// The move completed end-to-end, so reads and writes were issued and
		// acknowledged.
		Expect(insideStorage.Read(0, 4096)).To(Equal(data))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))

		reqInID := rec.taskID("req_in")
		Expect(reqInID).ToNot(BeZero())

		reqInMs := rec.milestonesOn(reqInID)

		// The src-read waits are charged as data milestones on the src port.
		var hasData, hasSubTask bool
		for _, ms := range reqInMs {
			if ms.Kind == tracing.MilestoneKindData &&
				ms.What == dataMover.GetPortByName("Outside").Name() {
				hasData = true
			}
			if ms.Kind == tracing.MilestoneKindSubTask &&
				ms.What == dataMover.GetPortByName("Inside").Name() {
				hasSubTask = true
			}
		}
		Expect(hasData).To(BeTrue(),
			"req_in should carry a data milestone for the src read wait")
		Expect(hasSubTask).To(BeTrue(),
			"req_in should carry a subtask milestone for the dst write wait")
	})

	It("waits at the head and admits a request that arrives while a "+
		"transaction is running, instead of dropping it", func() {
		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		makeMove := func() datamoverprotocol.DataMoveRequest {
			req := datamoverprotocol.DataMoveRequest{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = srcPort.AsRemote()
			req.Dst = topPort.AsRemote()
			req.SrcAddress = 0
			req.SrcSide = "outside"
			req.DstAddress = 0
			req.DstSide = "inside"
			req.ByteSize = 4096
			req.TrafficClass = "datamoverprotocol.DataMoveRequest"
			return req
		}

		// Both requests are delivered into the Top buffer before the data mover
		// runs, so the second sits behind the first (which becomes active first).
		// Before the reorder fix the data mover would retrieve-and-drop the
		// second while the first was active, silently losing it.
		req1 := makeMove()
		req2 := makeMove()
		topPort.Deliver(req1)
		topPort.Deliver(req2)

		engine.Run()

		// Both transactions completed: two responses come back to the source.
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))
		Expect(srcPort.RetrieveIncoming()).To(
			BeAssignableToTypeOf(datamoverprotocol.DataMoveResponse{}))

		// Two req_in tasks opened and both were closed (one per request), so the
		// second request was processed, not dropped.
		var reqInStarts, reqInEnds int
		reqInIDs := map[uint64]bool{}
		for _, s := range rec.starts {
			if s.Kind == "req_in" {
				reqInStarts++
				reqInIDs[s.ID] = true
			}
		}
		for _, e := range rec.endedIDs() {
			if reqInIDs[e] {
				reqInEnds++
			}
		}
		Expect(reqInStarts).To(Equal(2))
		Expect(reqInEnds).To(Equal(2))
	})
})
