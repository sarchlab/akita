package endpoint

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/noc/messaging"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

type sampleMsg struct {
	modeling.MsgMeta
}

func (m sampleMsg) Meta() modeling.MsgMeta {
	return m.MsgMeta
}

func (m sampleMsg) Clone() modeling.Msg {
	return m
}

var _ = Describe("End Point", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		sim               *MockSimulation
		devicePort        *MockPort
		networkPort       *MockPort
		defaultSwitchPort *MockPort
		endPoint          *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)

		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().GetEngine().Return(engine).AnyTimes()
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

		devicePort = NewMockPort(mockCtrl)
		devicePort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("DevicePort")).
			AnyTimes()
		networkPort = NewMockPort(mockCtrl)
		networkPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("NetworkPort")).
			AnyTimes()
		defaultSwitchPort = NewMockPort(mockCtrl)
		defaultSwitchPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("DefaultSwitchPort")).
			AnyTimes()

		devicePort.EXPECT().SetConnection(gomock.Any())

		endPoint = MakeBuilder().
			WithSimulation(sim).
			WithFreq(1).
			WithFlitByteSize(32).
			WithDevicePorts([]modeling.Port{devicePort}).
			Build("EndPoint")
		endPoint.NetworkPort = networkPort
		endPoint.DefaultSwitchDst = defaultSwitchPort.AsRemote()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send flits", func() {
		msg := &sampleMsg{}
		msg.Src = devicePort.AsRemote()
		msg.TrafficBytes = 33

		networkPort.EXPECT().PeekIncoming().Return(nil).AnyTimes()

		devicePort.EXPECT().PeekOutgoing().Return(msg)
		devicePort.EXPECT().RetrieveOutgoing().Return(msg)
		devicePort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()

		madeProgress := endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flit *messaging.Flit) {
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.SeqID).To(Equal(0))
			Expect(flit.NumFlitInMsg).To(Equal(2))
			Expect(flit.Msg).To(BeIdenticalTo(msg))
		})
		devicePort.EXPECT().NotifyAvailable()

		madeProgress = endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flit *messaging.Flit) {
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.SeqID).To(Equal(1))
			Expect(flit.NumFlitInMsg).To(Equal(2))
			Expect(flit.Msg).To(BeIdenticalTo(msg))
		})

		madeProgress = endPoint.Tick()

		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive message", func() {
		msg := &sampleMsg{}
		msg.Dst = devicePort.AsRemote()

		flit0 := messaging.FlitBuilder{}.
			WithSeqID(0).
			WithNumFlitInMsg(2).
			WithMsg(msg).
			Build()
		flit1 := messaging.FlitBuilder{}.
			WithSeqID(1).
			WithNumFlitInMsg(2).
			WithMsg(msg).
			Build()

		networkPort.EXPECT().PeekIncoming().Return(flit0)
		networkPort.EXPECT().PeekIncoming().Return(flit1)
		networkPort.EXPECT().PeekIncoming().Return(nil).Times(3)
		networkPort.EXPECT().RetrieveIncoming().Times(2)
		devicePort.EXPECT().Deliver(msg)
		devicePort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()

		madeProgress := endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick()
		Expect(madeProgress).To(BeFalse())
	})
})
