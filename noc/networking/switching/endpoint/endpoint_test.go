package endpoint

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
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
			Return(messaging.RemotePort("DevicePort")).
			AnyTimes()
		networkPort = NewMockPort(mockCtrl)
		networkPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("NetworkPort")).
			AnyTimes()
		defaultSwitchPort = NewMockPort(mockCtrl)
		defaultSwitchPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("DefaultSwitchPort")).
			AnyTimes()

		devicePort.EXPECT().SetConnection(gomock.Any())

		spec := DefaultSpec()
		spec.Freq = 1
		spec.FlitByteSize = 32

		endPoint = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{DevicePorts: []messaging.Port{devicePort}}).
			Build("EndPoint")
		endPoint.SetNetworkPort(networkPort)
		endPoint.SetDefaultSwitchDst(defaultSwitchPort.AsRemote())
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send flits", func() {
		msg := &messaging.MsgMeta{
			ID:           timing.GetIDGenerator().Generate(),
			Src:          devicePort.AsRemote(),
			TrafficBytes: 33,
		}

		networkPort.EXPECT().PeekIncoming().Return(nil).AnyTimes()

		devicePort.EXPECT().PeekOutgoing().Return(msg)
		devicePort.EXPECT().RetrieveOutgoing().Return(msg)
		devicePort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()

		madeProgress := endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(msg messaging.Msg) {
			flit := msg.(*packetization.Flit)
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.SeqID).To(Equal(0))
			Expect(flit.NumFlitInMsg).To(Equal(2))
		})
		devicePort.EXPECT().NotifyAvailable()

		madeProgress = endPoint.Tick()
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(msg messaging.Msg) {
			flit := msg.(*packetization.Flit)
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.SeqID).To(Equal(1))
			Expect(flit.NumFlitInMsg).To(Equal(2))
		})

		madeProgress = endPoint.Tick()

		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive message", func() {
		msg := &messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Dst: devicePort.AsRemote(),
		}

		flit0 := &packetization.Flit{}
		flit0.ID = timing.GetIDGenerator().Generate()
		flit0.TrafficClass = reflect.TypeOf(msg).String()
		flit0.SeqID = 0
		flit0.NumFlitInMsg = 2
		flit0.Msg = *msg
		flit1 := &packetization.Flit{}
		flit1.ID = timing.GetIDGenerator().Generate()
		flit1.TrafficClass = reflect.TypeOf(msg).String()
		flit1.SeqID = 1
		flit1.NumFlitInMsg = 2
		flit1.Msg = *msg

		networkPort.EXPECT().PeekIncoming().Return(flit0)
		networkPort.EXPECT().PeekIncoming().Return(flit1)
		networkPort.EXPECT().PeekIncoming().Return(nil).Times(3)
		networkPort.EXPECT().RetrieveIncoming().Times(2)
		devicePort.EXPECT().CanDeliver().Return(true)
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
