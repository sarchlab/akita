package switches

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Switch", func() {
	var (
		mockCtrl     *gomock.Controller
		engine       *MockEngine
		port1, port2 *MockPort
		dstPort      *MockPort
		routingTable *MockTable
		arbiter      *MockArbiter
		sw           *Comp
		swMiddleware *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)

		port1 = NewMockPort(mockCtrl)
		port1.EXPECT().AsRemote().
			Return(sim.RemotePort("LocalPort1")).
			AnyTimes()
		port1.EXPECT().Name().
			Return("LocalPort1").
			AnyTimes()

		port2 = NewMockPort(mockCtrl)
		port2.EXPECT().AsRemote().
			Return(sim.RemotePort("LocalPort2")).
			AnyTimes()
		port2.EXPECT().Name().
			Return("LocalPort2").
			AnyTimes()

		remote1 := NewMockPort(mockCtrl)
		remote1.EXPECT().AsRemote().
			Return(sim.RemotePort("RemotePort1")).
			AnyTimes()

		remote2 := NewMockPort(mockCtrl)
		remote2.EXPECT().AsRemote().
			Return(sim.RemotePort("RemotePort2")).
			AnyTimes()

		dstPort = NewMockPort(mockCtrl)
		dstPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("DstPort")).
			AnyTimes()

		routingTable = NewMockTable(mockCtrl)
		arbiter = NewMockArbiter(mockCtrl)
		arbiter.EXPECT().AddBuffer(gomock.Any()).AnyTimes()

		sw = MakeBuilder().
			WithEngine(engine).
			WithFreq(1).
			WithRoutingTable(routingTable).
			WithArbiter(arbiter).
			Build("Switch")

		pcs1 := portComplexState{
			LocalPortName:    "LocalPort1",
			RemotePort:       remote1.AsRemote(),
			NumInputChannel:  1,
			NumOutputChannel: 1,
			Latency:          1,
			PipelineWidth:    1,
		}
		sw.mw.addPort(port1, remote1.AsRemote(), pcs1)

		pcs2 := portComplexState{
			LocalPortName:    "LocalPort2",
			RemotePort:       remote2.AsRemote(),
			NumInputChannel:  1,
			NumOutputChannel: 1,
			Latency:          1,
			PipelineWidth:    1,
		}
		sw.mw.addPort(port2, remote2.AsRemote(), pcs2)

		swMiddleware = sw.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should start processing", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.Dst = port1.AsRemote()
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg

		port1.EXPECT().PeekIncoming().Return(flit)
		port1.EXPECT().RetrieveIncoming()
		port2.EXPECT().PeekIncoming().Return(nil)

		swMiddleware.updateAdapterPointers()
		madeProgress := swMiddleware.startProcessing()

		Expect(madeProgress).To(BeTrue())
		// Verify flit was accepted into pipeline
		next := sw.GetNextState()
		Expect(next.PortComplexes[0].PipelineStages).To(HaveLen(1))
		Expect(next.PortComplexes[0].PipelineStages[0].Item.Flit.ID).To(Equal(flit.ID))
	})

	It("should not start processing if pipeline is busy", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.Dst = port1.AsRemote()
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg

		// Fill pipeline so it can't accept
		next := sw.GetNextState()
		next.PortComplexes[0].PipelineStages = []pipelineStageState{
			{Lane: 0, Stage: 0, Item: flitPipelineItemState{TaskID: "t", Flit: sim.MsgMeta{}}},
		}

		port1.EXPECT().PeekIncoming().Return(flit)
		port2.EXPECT().PeekIncoming().Return(nil)

		swMiddleware.updateAdapterPointers()
		madeProgress := swMiddleware.startProcessing()

		Expect(madeProgress).To(BeFalse())
	})

	It("should tick the pipelines", func() {
		// Place an item in pipeline stage 0 for port1
		next := sw.GetNextState()
		next.PortComplexes[0].PipelineStages = []pipelineStageState{
			{Lane: 0, Stage: 0, Item: flitPipelineItemState{TaskID: "t1", Flit: sim.MsgMeta{ID: "flit1"}}},
		}

		swMiddleware.updateAdapterPointers()
		madeProgress := swMiddleware.movePipeline()

		Expect(madeProgress).To(BeTrue())
		// For latency=1, the item should have moved to RouteBuffer
		next = sw.GetNextState()
		Expect(next.PortComplexes[0].PipelineStages).To(HaveLen(0))
		Expect(next.PortComplexes[0].RouteBuffer).To(HaveLen(1))
	})

	It("should route", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg

		// Place item in route buffer for port1
		next := sw.GetNextState()
		next.PortComplexes[0].RouteBuffer = []flitPipelineItemState{
			{TaskID: "flit", Flit: flit.MsgMeta, MsgDst: dstPort.AsRemote()},
		}

		routingTable.EXPECT().
			FindPort(dstPort.AsRemote()).
			Return(port2.AsRemote())

		swMiddleware.updateAdapterPointers()
		madeProgress := swMiddleware.route()

		Expect(madeProgress).To(BeTrue())
		next = sw.GetNextState()
		Expect(next.PortComplexes[0].RouteBuffer).To(HaveLen(0))
		Expect(next.PortComplexes[0].ForwardBuffer).To(HaveLen(1))
	})

	It("should not route if forward buffer is full", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg

		// Place item in route buffer and fill forward buffer
		next := sw.GetNextState()
		next.PortComplexes[0].RouteBuffer = []flitPipelineItemState{
			{TaskID: "flit", Flit: flit.MsgMeta, MsgDst: dstPort.AsRemote()},
		}
		next.PortComplexes[0].ForwardBuffer = []forwardBufferEntry{
			{Flit: sim.MsgMeta{ID: "existing"}, OutputBufIdx: 0},
		}

		swMiddleware.updateAdapterPointers()
		madeProgress := swMiddleware.route()

		Expect(madeProgress).To(BeFalse())
	})

	It("should forward", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg
		// Place flit in forward buffer of port1, targeting sendOutBuffer of port2
		next := sw.GetNextState()
		next.PortComplexes[0].ForwardBuffer = []forwardBufferEntry{
			{Flit: flit.MsgMeta, OutputBufIdx: 1},
		}

		swMiddleware.updateAdapterPointers()

		arbiter.EXPECT().
			Arbitrate().
			Return([]queueing.Buffer{
				swMiddleware.forwardBufAdapters[0],
				swMiddleware.forwardBufAdapters[1],
			})

		madeProgress := swMiddleware.forward()

		Expect(madeProgress).To(BeTrue())
		next = sw.GetNextState()
		Expect(next.PortComplexes[0].ForwardBuffer).To(HaveLen(0))
		Expect(next.PortComplexes[1].SendOutBuffer).To(HaveLen(1))
	})

	It("should not forward if the output buffer is busy", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg
		// Fill sendOut buffer to capacity, forward buffer targets port2
		next := sw.GetNextState()
		next.PortComplexes[0].ForwardBuffer = []forwardBufferEntry{
			{Flit: flit.MsgMeta, OutputBufIdx: 1},
		}
		next.PortComplexes[1].SendOutBuffer = []sim.MsgMeta{{ID: "full"}}

		swMiddleware.updateAdapterPointers()

		arbiter.EXPECT().
			Arbitrate().
			Return([]queueing.Buffer{
				swMiddleware.forwardBufAdapters[0],
				swMiddleware.forwardBufAdapters[1],
			})

		madeProgress := swMiddleware.forward()

		Expect(madeProgress).To(BeFalse())
	})

	It("should send flits out", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg

		// Place flit in sendOutBuffer of port2
		next := sw.GetNextState()
		next.PortComplexes[1].SendOutBuffer = []sim.MsgMeta{flit.MsgMeta}
		sw.SetState(*next)

		port2.EXPECT().Send(gomock.Any()).Return(nil)

		swMiddleware.updateAdapterPointers()
		madeProgress := swMiddleware.sendOut()

		Expect(madeProgress).To(BeTrue())
		next = sw.GetNextState()
		Expect(next.PortComplexes[1].SendOutBuffer).To(HaveLen(0))
	})

	It("should wait if port is busy sending flits out", func() {
		msg := &sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: dstPort.AsRemote(),
			Dst: dstPort.AsRemote(),
		}
		flit := &messaging.Flit{}
		flit.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit.TrafficClass = reflect.TypeOf(msg).String()
		flit.Msg = msg

		// Place flit in sendOutBuffer of port2
		next := sw.GetNextState()
		next.PortComplexes[1].SendOutBuffer = []sim.MsgMeta{flit.MsgMeta}
		sw.SetState(*next)

		port2.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

		swMiddleware.updateAdapterPointers()
		madeProgress := swMiddleware.sendOut()

		Expect(madeProgress).To(BeFalse())
		// Flit should still be in send buffer
		next = sw.GetNextState()
		Expect(next.PortComplexes[1].SendOutBuffer).To(HaveLen(1))
	})
})
