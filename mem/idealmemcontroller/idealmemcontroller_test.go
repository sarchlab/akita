package idealmemcontroller

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v4/mem/mem"
	"go.uber.org/mock/gomock"

	"github.com/sarchlab/akita/v4/sim"

	. "github.com/onsi/gomega"
)

var _ = Describe("Ideal Memory Controller", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		memController *Comp
		port          *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		port = NewMockPort(mockCtrl)
		port.EXPECT().
			AsRemote().
			Return(sim.RemotePort("Port")).
			AnyTimes()

		memController = MakeBuilder().
			WithEngine(engine).
			WithNewStorage(1 * mem.MB).
			Build("MemCtrl")
		memController.Freq = 1000 * sim.MHz
		memController.Latency = 10
		memController.topPort = port
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should process read request", func() {
		readReq := mem.ReadReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithByteSize(4).
			Build()
		port.EXPECT().RetrieveIncoming().Return(readReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		madeProgress := memController.Tick()

		Expect(madeProgress).To(BeTrue())
	})

	It("should process write request", func() {
		writeReq := mem.WriteReqBuilder{}.
			WithDst(memController.topPort.AsRemote()).
			WithAddress(0).
			WithData([]byte{0, 1, 2, 3}).
			WithDirtyMask([]bool{false, false, true, false}).
			Build()
		port.EXPECT().RetrieveIncoming().Return(writeReq)
		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		madeProgress := memController.Tick()
		Expect(madeProgress).To(BeTrue())
	})
})
