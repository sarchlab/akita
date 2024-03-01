package endpoint

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/noc/messaging"
	"github.com/sarchlab/akita/v4/sim"
)

type sampleMsg struct {
	sim.MsgMeta
}

func (m *sampleMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

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
		networkPort = NewMockPort(mockCtrl)
		defaultSwitchPort = NewMockPort(mockCtrl)

		devicePort.EXPECT().SetConnection(gomock.Any())

		endPoint = MakeBuilder().
			WithEngine(engine).
			WithFreq(1).
			WithFlitByteSize(32).
			WithDevicePorts([]sim.Port{devicePort}).
			Build("EndPoint")
		endPoint.NetworkPort = networkPort
		endPoint.DefaultSwitchDst = defaultSwitchPort
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send flits", func() {
		msg := &sampleMsg{}
		msg.TrafficBytes = 33

		networkPort.EXPECT().PeekIncoming().Return(nil).AnyTimes()

		engine.EXPECT().Schedule(gomock.Any())
		msg.Src.Send(msg)

		madeProgress := endPoint.Tick(10)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flit *messaging.Flit) {
			Expect(flit.SendTime).To(Equal(sim.VTimeInSec(11)))
			Expect(flit.Src).To(Equal(networkPort))
			Expect(flit.Dst).To(Equal(defaultSwitchPort))
			Expect(flit.SeqID).To(Equal(0))
			Expect(flit.NumFlitInMsg).To(Equal(2))
			Expect(flit.Msg).To(BeIdenticalTo(msg))
		})
		devicePort.EXPECT().NotifyAvailable(gomock.Any())

		madeProgress = endPoint.Tick(11)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flit *messaging.Flit) {
			Expect(flit.SendTime).To(Equal(sim.VTimeInSec(12)))
			Expect(flit.Src).To(Equal(networkPort))
			Expect(flit.Dst).To(Equal(defaultSwitchPort))
			Expect(flit.SeqID).To(Equal(1))
			Expect(flit.NumFlitInMsg).To(Equal(2))
			Expect(flit.Msg).To(BeIdenticalTo(msg))
		})

		madeProgress = endPoint.Tick(12)

		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(13)

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive message", func() {
		msg := &sampleMsg{}
		msg.Dst = devicePort

		flit0 := messaging.FlitBuilder{}.
			WithSeqID(0).
			WithNumFlitInMsg(2).
			WithMsg(msg).
			Build()
		flit1 := messaging.FlitBuilder{}.
			WithSeqID(0).
			WithNumFlitInMsg(2).
			WithMsg(msg).
			Build()

		networkPort.EXPECT().PeekIncoming().Return(flit0)
		networkPort.EXPECT().PeekIncoming().Return(flit1)
		networkPort.EXPECT().PeekIncoming().Return(nil).Times(3)
		networkPort.EXPECT().RetrieveIncoming(gomock.Any()).Times(2)
		devicePort.EXPECT().Deliver(msg)

		madeProgress := endPoint.Tick(10)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(11)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(12)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(13)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(14)
		Expect(madeProgress).To(BeFalse())
	})
})
