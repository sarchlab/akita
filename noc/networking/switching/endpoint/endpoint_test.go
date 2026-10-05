package endpoint

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/akita/v5/messaging"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("End Point", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		sim               timing.Simulation
		devicePort        *MockPort
		networkPort       *MockPort
		defaultSwitchPort *MockPort
		endPoint          *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		engine.EXPECT().RegisterHandler(gomock.Any(), gomock.Any()).AnyTimes()
		engine.EXPECT().CurrentTime().Return(timing.VTimeInPicoSec(0)).AnyTimes()
		sim = modeling.NewStandaloneSimulation(engine)
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
		networkPort.EXPECT().Name().Return("EndPoint.NetworkPort").AnyTimes()
		networkPort.EXPECT().Owner().Return(nil)
		networkPort.EXPECT().SetOwner(gomock.Any())

		spec := Definition.DefaultSpec
		spec.Freq = 1
		spec.FlitByteSize = 32
		spec.DefaultSwitchDst = defaultSwitchPort.AsRemote()

		endPoint = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{DevicePorts: []messaging.Port{devicePort}}).
			WithPorts(Ports{NetworkPort: networkPort}).
			Build("EndPoint")
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send flits", func() {
		msg := messaging.MsgMeta{
			ID:           sim.NewID(),
			Src:          devicePort.AsRemote(),
			TrafficBytes: 33,
		}

		networkPort.EXPECT().PeekIncoming().Return(nil, false).AnyTimes()

		devicePort.EXPECT().PeekOutgoing().Return(msg, true)
		devicePort.EXPECT().RetrieveOutgoing().Return(msg, true)
		devicePort.EXPECT().PeekOutgoing().Return(nil, false).AnyTimes()

		madeProgress := modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(msg messaging.Msg) {
			flit := msg.(packetization.Flit)
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.SeqID).To(Equal(0))
			Expect(flit.NumFlitInMsg).To(Equal(2))
		})
		devicePort.EXPECT().NotifyAvailable()

		madeProgress = modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(msg messaging.Msg) {
			flit := msg.(packetization.Flit)
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.SeqID).To(Equal(1))
			Expect(flit.NumFlitInMsg).To(Equal(2))
		})

		madeProgress = modelingtest.Tick(endPoint)

		Expect(madeProgress).To(BeTrue())

		madeProgress = modelingtest.Tick(endPoint)

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive message", func() {
		msg := messaging.MsgMeta{
			ID:  sim.NewID(),
			Dst: devicePort.AsRemote(),
		}

		flit0 := packetization.Flit{}
		flit0.ID = sim.NewID()
		flit0.TrafficClass = reflect.TypeOf(msg).String()
		flit0.SeqID = 0
		flit0.NumFlitInMsg = 2
		flit0.Msg = msg
		flit1 := packetization.Flit{}
		flit1.ID = sim.NewID()
		flit1.TrafficClass = reflect.TypeOf(msg).String()
		flit1.SeqID = 1
		flit1.NumFlitInMsg = 2
		flit1.Msg = msg

		networkPort.EXPECT().PeekIncoming().Return(flit0, true)
		networkPort.EXPECT().PeekIncoming().Return(flit1, true)
		networkPort.EXPECT().PeekIncoming().Return(nil, false).Times(3)
		networkPort.EXPECT().RetrieveIncoming().Times(2)
		devicePort.EXPECT().CanDeliver().Return(true)
		devicePort.EXPECT().Deliver(packetization.AssembledMsg{MsgMeta: msg})
		devicePort.EXPECT().PeekOutgoing().Return(nil, false).AnyTimes()

		madeProgress := modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		madeProgress = modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		madeProgress = modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		madeProgress = modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		madeProgress = modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeFalse())
	})
})
