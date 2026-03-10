package endpoint

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("End Point", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		devicePort        *MockPort
		networkPort       *MockPort
		defaultSwitchPort *MockPort
		endPoint          *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		devicePort = NewMockPort(mockCtrl)
		devicePort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("DevicePort")).
			AnyTimes()
		networkPort = NewMockPort(mockCtrl)
		networkPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("NetworkPort")).
			AnyTimes()
		defaultSwitchPort = NewMockPort(mockCtrl)
		defaultSwitchPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("DefaultSwitchPort")).
			AnyTimes()

		devicePort.EXPECT().SetConnection(gomock.Any())

		endPoint = MakeBuilder().
			WithEngine(engine).
			WithFreq(1).
			WithFlitByteSize(32).
			WithDevicePorts([]sim.Port{devicePort}).
			WithNetworkPort(sim.NewPort(nil, 4, 4, "EndPoint.NetworkPort")).
			Build("EndPoint")
		endPoint.NetworkPort = networkPort
		endPoint.DefaultSwitchDst = defaultSwitchPort.AsRemote()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send flits", func() {
		msg := &sim.Msg{
			MsgMeta: sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
				Src:          devicePort.AsRemote(),
				TrafficBytes: 33,
			},
		}

		networkPort.EXPECT().PeekIncoming().Return(nil).AnyTimes()

		devicePort.EXPECT().PeekOutgoing().Return(msg)
		devicePort.EXPECT().RetrieveOutgoing().Return(msg)
		devicePort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()

		madeProgress := endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flitMsg *sim.Msg) {
			flitPayload := sim.MsgPayload[messaging.FlitPayload](flitMsg)
			Expect(flitMsg.Src).To(Equal(networkPort.AsRemote()))
			Expect(flitMsg.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flitPayload.SeqID).To(Equal(0))
			Expect(flitPayload.NumFlitInMsg).To(Equal(2))
			Expect(flitPayload.Msg).To(BeIdenticalTo(msg))
		})
		devicePort.EXPECT().NotifyAvailable()

		madeProgress = endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flitMsg *sim.Msg) {
			flitPayload := sim.MsgPayload[messaging.FlitPayload](flitMsg)
			Expect(flitMsg.Src).To(Equal(networkPort.AsRemote()))
			Expect(flitMsg.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flitPayload.SeqID).To(Equal(1))
			Expect(flitPayload.NumFlitInMsg).To(Equal(2))
			Expect(flitPayload.Msg).To(BeIdenticalTo(msg))
		})

		madeProgress = endPoint.Tick()

		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive message", func() {
		msg := &sim.Msg{
			MsgMeta: sim.MsgMeta{
				ID:  sim.GetIDGenerator().Generate(),
				Dst: devicePort.AsRemote(),
			},
		}

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
