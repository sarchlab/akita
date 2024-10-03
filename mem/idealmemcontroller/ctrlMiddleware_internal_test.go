package idealmemcontroller

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

var _ = FDescribe("CtrlMiddleware", func() {
	var (
		mockCtrl       *gomock.Controller
		comp           *Comp
		engine         *MockEngine
		remoteCtrlPort *MockPort
		ctrlPort       *MockPort
		ctrlMW         *ctrlMiddleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)
		remoteCtrlPort = NewMockPort(mockCtrl)

		comp = MakeBuilder().
			WithEngine(engine).
			WithNewStorage(1 * mem.MB).
			Build("MemCtrl")
		comp.Freq = 1000 * sim.MHz
		comp.Latency = 10
		comp.ctrlPort = ctrlPort

		ctrlMW = &ctrlMiddleware{
			Comp: comp,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no ctrl message", func() {
		ctrlPort.EXPECT().RetrieveIncoming().Return(nil)

		madeProgress := ctrlMW.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should handle enable message", func() {
		comp.state = "paused"

		ctrlMsg := mem.ControlMsgBuilder{}.
			WithSrc(remoteCtrlPort).
			WithDst(ctrlPort).
			WithCtrlInfo(true, false, false, false).
			Build()
		ctrlPort.EXPECT().RetrieveIncoming().Return(ctrlMsg)
		ctrlPort.EXPECT().
			Send(gomock.Any()).
			Do(func(msg *sim.GeneralRsp) {
				Expect(msg.Src).To(Equal(ctrlPort))
				Expect(msg.Dst).To(Equal(remoteCtrlPort))
				Expect(msg.OriginalReq).To(Equal(ctrlMsg))
			}).
			Return(nil).
			AnyTimes()

		madeProgress := ctrlMW.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(comp.state).To(Equal("enable"))
	})

	It("should handle pause message", func() {
		comp.state = "enable"

		ctrlMsg := mem.ControlMsgBuilder{}.
			WithSrc(remoteCtrlPort).
			WithDst(ctrlPort).
			WithCtrlInfo(false, false, false, false).
			Build()
		ctrlPort.EXPECT().RetrieveIncoming().Return(ctrlMsg)
		ctrlPort.EXPECT().
			Send(gomock.Any()).
			Do(func(msg *sim.GeneralRsp) {
				Expect(msg.Src).To(Equal(ctrlPort))
				Expect(msg.Dst).To(Equal(remoteCtrlPort))
				Expect(msg.OriginalReq).To(Equal(ctrlMsg))
			}).
			Return(nil).
			AnyTimes()

		madeProgress := ctrlMW.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(comp.state).To(Equal("pause"))
	})

	It("should handle drain message", func() {
		comp.state = "enable"

		ctrlMsg := mem.ControlMsgBuilder{}.
			WithSrc(remoteCtrlPort).
			WithDst(ctrlPort).
			WithCtrlInfo(false, true, false, false).
			Build()
		ctrlPort.EXPECT().RetrieveIncoming().Return(ctrlMsg)
		madeProgress := ctrlMW.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(comp.state).To(Equal("drain"))
		Expect(comp.respondReq).To(Equal(ctrlMsg))
	})

})
