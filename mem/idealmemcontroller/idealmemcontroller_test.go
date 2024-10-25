package idealmemcontroller

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v4/mem/mem"

	"github.com/sarchlab/akita/v4/sim"

	. "github.com/onsi/gomega"
)

var _ = Describe("Ideal Memory Controller", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		memController *Comp
		port          *MockPort
		ctrlPort      *MockPort
		// ctrlMW        *ctrlMiddleware
		// FuncMW        *funcMiddleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		port = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)

		memController = MakeBuilder().
			WithEngine(engine).
			WithNewStorage(1 * mem.MB).
			Build("MemCtrl")
		memController.Freq = 1000 * sim.MHz
		memController.Latency = 10
		memController.topPort = port
		memController.ctrlPort = ctrlPort
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should process read request", func() {
		readReq := mem.ReadReqBuilder{}.
			WithDst(memController.topPort).
			WithAddress(0).
			WithByteSize(4).
			Build()
		ctrlPort.EXPECT().PeekIncoming().Return(nil)
		port.EXPECT().RetrieveIncoming().Return(readReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		madeProgress := memController.Tick()

		Expect(madeProgress).To(BeTrue())
	})

	It("should process write request", func() {
		writeReq := mem.WriteReqBuilder{}.
			WithDst(memController.topPort).
			WithAddress(0).
			WithData([]byte{0, 1, 2, 3}).
			WithDirtyMask([]bool{false, false, true, false}).
			Build()
		ctrlPort.EXPECT().PeekIncoming().Return(nil)
		port.EXPECT().RetrieveIncoming().Return(writeReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
	})

	It("should process pause request", func() {
		ctrlMsg := mem.ControlMsgBuilder{}.
			WithSrc(ctrlPort).
			WithDst(ctrlPort).
			WithCtrlInfo(false, false, false, false).
			Build()
		ctrlPort.EXPECT().PeekIncoming().Return(ctrlMsg)
		ctrlPort.EXPECT().RetrieveIncoming().Return(ctrlMsg)
		ctrlPort.EXPECT().
			Send(gomock.Any()).
			Do(func(msg *sim.GeneralRsp) {
				Expect(msg.Src).To(Equal(ctrlPort))
				Expect(msg.Dst).To(Equal(ctrlPort))
				Expect(msg.OriginalReq).To(Equal(ctrlMsg))
			}).
			Return(nil).
			AnyTimes()

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
	})

	It("should process enable request", func() {
		memController.state = "pause"
		ctrlMsg := mem.ControlMsgBuilder{}.
			WithSrc(ctrlPort).
			WithDst(ctrlPort).
			WithCtrlInfo(true, false, false, false).
			Build()
		ctrlPort.EXPECT().PeekIncoming().Return(ctrlMsg)
		ctrlPort.EXPECT().RetrieveIncoming().Return(ctrlMsg)
		port.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
		ctrlPort.EXPECT().
			Send(gomock.Any()).
			Do(func(msg *sim.GeneralRsp) {
				Expect(msg.Src).To(Equal(ctrlPort))
				Expect(msg.Dst).To(Equal(ctrlPort))
				Expect(msg.OriginalReq).To(Equal(ctrlMsg))
			}).
			Return(nil).
			AnyTimes()

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
	})

	It("should process drain request", func() {
		ctrlMsg := mem.ControlMsgBuilder{}.
			WithSrc(ctrlPort).
			WithDst(ctrlPort).
			WithCtrlInfo(false, true, false, false).
			Build()
		ctrlPort.EXPECT().PeekIncoming().Return(ctrlMsg)
		ctrlPort.EXPECT().RetrieveIncoming().Return(ctrlMsg)
		port.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

		ctrlPort.EXPECT().
			Send(gomock.Any()).
			Do(func(msg *sim.GeneralRsp) {
				Expect(msg.Src).To(Equal(ctrlPort))
				Expect(msg.Dst).To(Equal(ctrlPort))
				Expect(msg.OriginalReq).To(Equal(ctrlMsg))
			}).
			Return(nil).
			AnyTimes()

		madeProgress := memController.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(memController.state).To(Equal("pause"))
	})
})
