package idealmemcontroller

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

var _ = Describe("FuncMiddleware", func() {
	var (
		mockCtrl *gomock.Controller
		comp     *Comp
		engine   *MockEngine
		ctrlPort *MockPort
		topPort  *MockPort
		memMW    *memMiddleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		topPort = NewMockPort(mockCtrl)

		comp = MakeBuilder().
			WithEngine(engine).
			WithNewStorage(1 * mem.MB).
			WithWidth(1).
			Build("MemCtrl")
		comp.Freq = 1000 * sim.MHz
		comp.Latency = 10
		comp.topPort = topPort
		comp.ctrlPort = ctrlPort

		memMW = &memMiddleware{
			Comp: comp,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no memory read request or memory store request",
		func() {
			topPort.EXPECT().RetrieveIncoming().Return(nil)

			madeProgress := memMW.Tick()

			Expect(madeProgress).To(BeTrue())
		})

	It("should handle memory read request", func() {
		readReq := mem.ReadReqBuilder{}.
			WithDst(memMW.topPort).
			WithAddress(0).
			WithByteSize(4).
			Build()
		topPort.EXPECT().RetrieveIncoming().Return(readReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		madeProgress := memMW.Tick()

		Expect(madeProgress).To(BeTrue())
	})

	It("should process write request", func() {
		writeReq := mem.WriteReqBuilder{}.
			WithDst(memMW.topPort).
			WithAddress(0).
			WithData([]byte{0, 1, 2, 3}).
			WithDirtyMask([]bool{false, false, true, false}).
			Build()
		topPort.EXPECT().RetrieveIncoming().Return(writeReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		madeProgress := memMW.Tick()
		Expect(madeProgress).To(BeTrue())
	})
})
