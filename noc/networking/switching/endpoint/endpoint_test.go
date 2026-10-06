package endpoint

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("End Point", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		sim               timing.Simulation
		devicePort        *twowaybuffered.Port
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
		devicePort = twowaybuffered.NewPort("DevicePort", 1, 1)
		devicePort.SetOwner(payloadOwner{"Device"})
		engine.EXPECT().Schedule(gomock.Any()).AnyTimes()
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
		msg := messaging.Msg{
			ID:           sim.NewID(),
			Src:          devicePort.AsRemote(),
			Dst:          "OtherDevice",
			TrafficBytes: 33,
			Payload:      memprotocol.WriteReq{Address: 64, Data: []byte{1, 2, 3}},
		}

		networkPort.EXPECT().PeekIncoming().Return(messaging.Msg{}, false).AnyTimes()

		devicePort.Send(msg)

		madeProgress := modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(sent messaging.Msg) {
			flit := sent
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.Payload.(packetization.Flit).SeqID).To(Equal(0))
			Expect(flit.Payload.(packetization.Flit).NumFlitInMsg).To(Equal(2))
			Expect(flit.Payload.(packetization.Flit).Msg).To(Equal(msg))
			Expect(flit.Payload.(packetization.Flit).MsgID).To(Equal(msg.ID))
		})

		madeProgress = modelingtest.Tick(endPoint)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().CanSend().Return(true)
		networkPort.EXPECT().Send(gomock.Any()).Do(func(sent messaging.Msg) {
			flit := sent
			Expect(flit.Src).To(Equal(networkPort.AsRemote()))
			Expect(flit.Dst).To(Equal(defaultSwitchPort.AsRemote()))
			Expect(flit.Payload.(packetization.Flit).SeqID).To(Equal(1))
			Expect(flit.Payload.(packetization.Flit).NumFlitInMsg).To(Equal(2))
			Expect(flit.Payload.(packetization.Flit).Msg.Payload).To(BeNil())
			Expect(flit.Payload.(packetization.Flit).MsgID).To(Equal(msg.ID))
		})

		madeProgress = modelingtest.Tick(endPoint)

		Expect(madeProgress).To(BeTrue())

		madeProgress = modelingtest.Tick(endPoint)

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive message", func() {
		msg := messaging.Msg{
			ID:  sim.NewID(),
			Dst: devicePort.AsRemote(),
		}

		flit0 := messaging.Msg{Payload: packetization.Flit{
			SeqID:        0,
			NumFlitInMsg: 2,
			MsgID:        msg.ID, Dst: msg.Dst, Msg: msg},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		flit1 := messaging.Msg{Payload: packetization.Flit{
			SeqID:        1,
			NumFlitInMsg: 2,
			MsgID:        msg.ID, Dst: msg.Dst},
			ID:           sim.NewID(),
			TrafficClass: "packetization.Flit"}

		networkPort.EXPECT().PeekIncoming().Return(flit0, true)
		networkPort.EXPECT().PeekIncoming().Return(flit1, true)
		networkPort.EXPECT().PeekIncoming().Return(messaging.Msg{}, false).Times(3)
		networkPort.EXPECT().RetrieveIncoming().Times(2)

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
		received, ok := devicePort.PeekIncoming()
		Expect(ok).To(BeTrue())
		Expect(received).To(Equal(msg))
	})
})
