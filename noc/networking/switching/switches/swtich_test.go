package switches

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/noc/messaging"
	"github.com/sarchlab/akita/v4/sim"
	gomock "go.uber.org/mock/gomock"
)

func createMockPortComplex(ctrl *gomock.Controller, index int) portComplex {
	local := NewMockPort(ctrl)
	local.EXPECT().AsRemote().
		Return(sim.RemotePort(fmt.Sprintf("LocalPort%d", index))).
		AnyTimes()

	remote := NewMockPort(ctrl)
	remote.EXPECT().AsRemote().
		Return(sim.RemotePort(fmt.Sprintf("RemotePort%d", index))).
		AnyTimes()

	routeBuf := NewMockBuffer(ctrl)
	forwardBuf := NewMockBuffer(ctrl)
	sendOutBuf := NewMockBuffer(ctrl)
	pipeline := NewMockPipeline(ctrl)

	pc := portComplex{
		localPort:        local,
		remotePort:       remote.AsRemote(),
		pipeline:         pipeline,
		routeBuffer:      routeBuf,
		forwardBuffer:    forwardBuf,
		sendOutBuffer:    sendOutBuf,
		numInputChannel:  1,
		numOutputChannel: 1,
	}

	return pc
}

type sampleMsg struct {
	sim.MsgMeta
}

func (m *sampleMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *sampleMsg) Clone() sim.Msg {
	return m
}

var _ = Describe("Switch", func() {
	var (
		mockCtrl                   *gomock.Controller
		engine                     *MockEngine
		portComplex1, portComplex2 portComplex
		dstPort                    *MockPort
		routingTable               *MockTable
		arbiter                    *MockArbiter
		sw                         *Comp
		swMiddleware               *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)

		portComplex1 = createMockPortComplex(mockCtrl, 1)
		portComplex2 = createMockPortComplex(mockCtrl, 2)

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
		sw.addPort(portComplex1)
		sw.addPort(portComplex2)
		swMiddleware = sw.Middlewares()[0].(*middleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should start processing", func() {
		port1 := portComplex1.localPort.(*MockPort)
		port2 := portComplex2.localPort.(*MockPort)
		port1Pipeline := portComplex1.pipeline.(*MockPipeline)

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithDst(port1.AsRemote()).
			WithMsg(msg).
			Build()

		port1.EXPECT().PeekIncoming().Return(flit)
		port1.EXPECT().RetrieveIncoming()
		port2.EXPECT().PeekIncoming().Return(nil)
		port1Pipeline.EXPECT().CanAccept().Return(true)
		port1Pipeline.
			EXPECT().
			Accept(gomock.Any()).
			Do(func(i flitPipelineItem) {
				Expect(i.flit).To(Equal(flit))
			})

		madeProgress := swMiddleware.startProcessing()

		Expect(madeProgress).To(BeTrue())
	})

	It("should not start processing if pipeline is busy", func() {
		port1 := portComplex1.localPort.(*MockPort)
		port2 := portComplex2.localPort.(*MockPort)
		port1Pipeline := portComplex1.pipeline.(*MockPipeline)

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithDst(port1.AsRemote()).
			WithMsg(msg).
			Build()

		port1.EXPECT().PeekIncoming().Return(flit)
		port2.EXPECT().PeekIncoming().Return(nil)
		port1Pipeline.EXPECT().CanAccept().Return(false)

		madeProgress := swMiddleware.startProcessing()

		Expect(madeProgress).To(BeFalse())
	})

	It("should tick the pipelines", func() {
		port1Pipeline := portComplex1.pipeline.(*MockPipeline)
		port2Pipeline := portComplex2.pipeline.(*MockPipeline)

		port1Pipeline.EXPECT().Tick().Return(false)
		port2Pipeline.EXPECT().Tick().Return(true)

		madeProgress := swMiddleware.movePipeline()

		Expect(madeProgress).To(BeTrue())
	})

	It("should route", func() {
		routeBuffer1 := portComplex1.routeBuffer.(*MockBuffer)
		routeBuffer2 := portComplex2.routeBuffer.(*MockBuffer)
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		pipelineItem := flitPipelineItem{taskID: "flit", flit: flit}
		routeBuffer1.EXPECT().Peek().Return(pipelineItem)
		routeBuffer1.EXPECT().Pop()
		routeBuffer2.EXPECT().Peek().Return(nil)
		forwardBuffer1.EXPECT().CanPush().Return(true)
		forwardBuffer1.EXPECT().Push(flit)
		routingTable.EXPECT().
			FindPort(dstPort.AsRemote()).
			Return(portComplex2.localPort.AsRemote())

		madeProgress := swMiddleware.route()

		Expect(madeProgress).To(BeTrue())
		Expect(flit.OutputBuf).To(BeIdenticalTo(portComplex2.sendOutBuffer))
	})

	It("should not route if forward buffer is full", func() {
		routeBuffer1 := portComplex1.routeBuffer.(*MockBuffer)
		routeBuffer2 := portComplex2.routeBuffer.(*MockBuffer)
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		pipelineItem := flitPipelineItem{taskID: "flit", flit: flit}
		routeBuffer1.EXPECT().Peek().Return(pipelineItem)
		routeBuffer2.EXPECT().Peek().Return(nil)
		forwardBuffer1.EXPECT().CanPush().Return(false)

		madeProgress := swMiddleware.route()

		Expect(madeProgress).To(BeFalse())
	})

	It("should forward", func() {
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)
		forwardBuffer2 := portComplex2.forwardBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()
		flit.OutputBuf = sendOutBuffer2

		arbiter.EXPECT().
			Arbitrate().
			Return([]sim.Buffer{forwardBuffer1, forwardBuffer2})
		forwardBuffer1.EXPECT().Peek().Return(flit)
		forwardBuffer1.EXPECT().Peek().Return(nil)
		forwardBuffer1.EXPECT().Pop()
		forwardBuffer2.EXPECT().Peek().Return(nil)
		sendOutBuffer2.EXPECT().CanPush().Return(true)
		sendOutBuffer2.EXPECT().Push(flit)

		madeProgress := swMiddleware.forward()

		Expect(madeProgress).To(BeTrue())
	})

	It("should not forward if the output buffer is busy", func() {
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)
		forwardBuffer2 := portComplex2.forwardBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()
		flit.OutputBuf = sendOutBuffer2

		arbiter.EXPECT().
			Arbitrate().
			Return([]sim.Buffer{forwardBuffer1, forwardBuffer2})
		forwardBuffer1.EXPECT().Peek().Return(flit)
		forwardBuffer2.EXPECT().Peek().Return(nil)
		sendOutBuffer2.EXPECT().CanPush().Return(false)

		madeProgress := swMiddleware.forward()

		Expect(madeProgress).To(BeFalse())
	})

	It("should send flits out", func() {
		sendOutBuffer1 := portComplex1.sendOutBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)
		localPort2 := portComplex2.localPort.(*MockPort)
		remotePort2 := portComplex2.remotePort

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		sendOutBuffer1.EXPECT().Peek().Return(nil).AnyTimes()
		sendOutBuffer2.EXPECT().Peek().Return(flit)
		sendOutBuffer2.EXPECT().Pop()
		localPort2.EXPECT().Send(flit).Return(nil)

		madeProgress := swMiddleware.sendOut()

		Expect(madeProgress).To(BeTrue())
		Expect(flit.Dst).To(Equal(remotePort2))
		Expect(flit.Src).To(Equal(portComplex2.localPort.AsRemote()))
	})

	It("should wait if port is busy flits out", func() {
		sendOutBuffer1 := portComplex1.sendOutBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)
		localPort2 := portComplex2.localPort.(*MockPort)
		remotePort2 := portComplex2.remotePort

		msg := &sampleMsg{}
		msg.Src = dstPort.AsRemote()
		msg.Dst = dstPort.AsRemote()
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		sendOutBuffer1.EXPECT().Peek().Return(nil).AnyTimes()
		sendOutBuffer2.EXPECT().Peek().Return(flit)
		localPort2.EXPECT().Send(flit).Return(&sim.SendError{})

		madeProgress := swMiddleware.sendOut()

		Expect(madeProgress).To(BeFalse())
		Expect(flit.Dst).To(Equal(remotePort2))
		Expect(flit.Src).To(Equal(portComplex2.localPort.AsRemote()))
	})
})
