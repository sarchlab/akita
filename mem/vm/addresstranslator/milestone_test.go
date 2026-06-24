package addresstranslator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// traceRecorder captures the full trace event stream the address translator
// emits — task starts/ends, milestones, and tags — in emission order and
// without the DBTracer's dedup, so a test can assert the exact ordered set of
// milestones and which req_out tasks are finalized.
type traceRecorder struct {
	tracing.NopTracer
	starts     []tracing.TaskStart
	ends       []tracing.TaskEnd
	milestones []tracing.Milestone
	tags       []tracing.TaskTag
}

func (r *traceRecorder) StartTask(t tracing.TaskStart) {
	r.starts = append(r.starts, t)
}

func (r *traceRecorder) EndTask(t tracing.TaskEnd) {
	r.ends = append(r.ends, t)
}

func (r *traceRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *traceRecorder) AddTaskTag(t tracing.TaskTag) {
	r.tags = append(r.tags, t)
}

func (r *traceRecorder) endedIDs() []uint64 {
	ids := make([]uint64, len(r.ends))
	for i, e := range r.ends {
		ids[i] = e.ID
	}

	return ids
}

// taskID returns the ID of the first recorded task start of the given kind.
func (r *traceRecorder) taskID(kind string) uint64 {
	for _, s := range r.starts {
		if s.Kind == kind {
			return s.ID
		}
	}

	return 0
}

func (r *traceRecorder) milestonesOn(taskID uint64) []tracing.Milestone {
	var ms []tracing.Milestone
	for _, m := range r.milestones {
		if m.TaskID == taskID {
			ms = append(ms, m)
		}
	}

	return ms
}

func (r *traceRecorder) kindsOn(taskID uint64) []tracing.MilestoneKind {
	ms := r.milestonesOn(taskID)
	ks := make([]tracing.MilestoneKind, len(ms))
	for i, m := range ms {
		ks[i] = m.Kind
	}

	return ks
}

var _ = Describe("Address Translator milestones", func() {
	var (
		engine          timing.Engine
		at              *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ptMW            *parseTranslateMW
		rpMW            *respondPipelineMW
		rec             *traceRecorder
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.Freq = 1

		resources := Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("MemPort"),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("TranslationProvider"),
			},
		}

		reg := modeling.NewStandaloneRegistrar(engine)
		at = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(resources).
			Build("AddressTranslator")

		assignPorts(reg, at, topBufSize)

		topPort = at.GetPortByName("Top")
		bottomPort = at.GetPortByName("Bottom")
		translationPort = at.GetPortByName("Translation")
		ctrlPort := at.GetPortByName("Control")

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}

		ptMW = at.Middlewares()[1].(*parseTranslateMW)
		rpMW = at.Middlewares()[2].(*respondPipelineMW)

		// Attach the recorder before driving so MsgIDAtReceiver hands out
		// real receiver-side task IDs (it returns 0 when there are no hooks).
		rec = &traceRecorder{}
		tracing.CollectTrace(at, rec)
		// The translation-send admission milestone lands on the Top-port buffer
		// task, so the test needs the buffer task to exist.
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

	makeWrite := func(addr uint64, data []byte) memprotocol.WriteReq {
		req := memprotocol.WriteReq{Address: addr, Data: data}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.TrafficBytes = len(data) + 12
		req.TrafficClass = "memprotocol.WriteReq"
		return req
	}

	// driveRoundTrip pushes one request through the whole translator: admit
	// and send the translation (translate), receive the translation and
	// forward downstream (parseTranslation), then receive the bottom response
	// and reply upstream (respond). bottomRsp builds the bottom-side response
	// given the translated request's ID.
	driveRoundTrip := func(
		req memprotocol.AccessReq,
		bottomRsp func(rspTo uint64) messaging.Msg,
	) {
		topPort.Deliver(req)
		Expect(ptMW.translate()).To(BeTrue())

		transReqID := at.State.Transactions[0].TranslationReqID
		translationPort.RetrieveOutgoing()

		transRsp := vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x20000},
		}
		transRsp.ID = timing.GetIDGenerator().Generate()
		transRsp.RspTo = transReqID
		transRsp.TrafficClass = "vmprotocol.TranslationRsp"
		translationPort.Deliver(transRsp)
		Expect(rpMW.parseTranslation()).To(BeTrue())

		bottomReqID := at.State.InflightReqToBottom[0].ReqToBottomID
		bottomPort.RetrieveOutgoing()

		bottomPort.Deliver(bottomRsp(bottomReqID))
		Expect(rpMW.respond()).To(BeTrue())

		topPort.RetrieveOutgoing()
	}

	It("splits admission (buffer task) from processing (req_in) for a read", func() {
		driveRoundTrip(makeRead(0x10040), func(rspTo uint64) messaging.Msg {
			rsp := memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}}
			rsp.ID = timing.GetIDGenerator().Generate()
			rsp.RspTo = rspTo
			rsp.TrafficClass = "memprotocol.DataReadyRsp"
			return rsp
		})

		bufID := rec.taskID(tracing.IncomingBufferTaskKind)
		reqInID := rec.taskID("req_in")
		Expect(bufID).ToNot(BeZero())
		Expect(reqInID).ToNot(BeZero())
		Expect(bufID).ToNot(Equal(reqInID))

		// The buffer task owns the pre-admission story: reached the head of the
		// Top buffer, then waited to send the translation request.
		Expect(rec.kindsOn(bufID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindQueue,
			tracing.MilestoneKindNetworkBusy,
		}))
		bufMs := rec.milestonesOn(bufID)
		Expect(bufMs[0].What).To(Equal(topPort.Name()))
		Expect(bufMs[1].What).To(Equal(translationPort.Name()))

		// req_in owns only the post-admission processing — including the bottom
		// network-busy milestone (on the top req_in, not a phantom keyed by the
		// downstream request's receiver ID).
		Expect(rec.kindsOn(reqInID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindTranslation,
			tracing.MilestoneKindNetworkBusy,
			tracing.MilestoneKindData,
			tracing.MilestoneKindNetworkBusy,
		}))
		reqMs := rec.milestonesOn(reqInID)
		Expect(reqMs[0].What).To(Equal("translation"))
		Expect(reqMs[1].What).To(Equal(bottomPort.Name()))
		Expect(reqMs[2].What).To(Equal("data"))
		Expect(reqMs[3].What).To(Equal(topPort.Name()))
	})

	It("distinguishes a write with a subtask milestone on req_in", func() {
		driveRoundTrip(
			makeWrite(0x10040, []byte{1, 2, 3, 4}),
			func(rspTo uint64) messaging.Msg {
				rsp := memprotocol.WriteDoneRsp{}
				rsp.ID = timing.GetIDGenerator().Generate()
				rsp.RspTo = rspTo
				rsp.TrafficClass = "memprotocol.WriteDoneRsp"
				return rsp
			},
		)

		reqInID := rec.taskID("req_in")
		Expect(rec.kindsOn(reqInID)).To(Equal([]tracing.MilestoneKind{
			tracing.MilestoneKindTranslation,
			tracing.MilestoneKindNetworkBusy,
			tracing.MilestoneKindSubTask,
			tracing.MilestoneKindNetworkBusy,
		}))
	})

	It("finalizes the completing transaction's translation req_out, "+
		"not a later transaction shifted into its slot", func() {
		// Two in-flight transactions; complete the first (non-last) one. The
		// removeTransaction append-shift must not redirect the trace finalize
		// to the surviving transaction shifted into the freed slot.
		transReq1ID := timing.GetIDGenerator().Generate()
		transReq2ID := timing.GetIDGenerator().Generate()

		req := makeRead(0x10040)

		at.State = State{
			Transactions: []transactionState{
				{
					TranslationReqID:  transReq1ID,
					TranslationReqSrc: translationPort.AsRemote(),
					TranslationReqDst: messaging.RemotePort("TranslationProvider"),
					IncomingReqs: []incomingReqState{
						msgToIncomingReqState(req),
					},
					TranslationDone: true,
				},
				{TranslationReqID: transReq2ID},
			},
		}

		transRsp := vmprotocol.TranslationRsp{
			Page: vm.Page{PID: 1, VAddr: 0x10000, PAddr: 0x20000},
		}
		transRsp.ID = timing.GetIDGenerator().Generate()
		transRsp.RspTo = transReq1ID
		transRsp.TrafficClass = "vmprotocol.TranslationRsp"
		translationPort.Deliver(transRsp)

		Expect(rpMW.parseTranslation()).To(BeTrue())

		Expect(rec.endedIDs()).To(ContainElement(transReq1ID))
		Expect(rec.endedIDs()).NotTo(ContainElement(transReq2ID))
	})
})
