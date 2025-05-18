package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/vm/tlb/internal"
	"go.uber.org/mock/gomock"
)

var _ = Describe("TLB", func() {

	var (
		mockCtrl    *gomock.Controller
		engine      *MockEngine
		comp        *Comp
		tlbMW       *tlbMiddleware
		ctrlMW      *ctrlMiddleware
		set         *MockSet
		topPort     *MockPort
		bottomPort  *MockPort
		controlPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		set = NewMockSet(mockCtrl)
		topPort = NewMockPort(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		controlPort = NewMockPort(mockCtrl)

		comp = MakeBuilder().WithEngine(engine).Build("TLB")
		comp.topPort = topPort
		comp.bottomPort = bottomPort
		comp.controlPort = controlPort
		comp.sets = []internal.Set{set}

		ctrlMW = comp.Middlewares()[0].(*ctrlMiddleware)
		tlbMW = comp.Middlewares()[1].(*tlbMiddleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no req in TopPort", func() {
		topPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := tlbMW.lookup()

		Expect(madeProgress).To(BeFalse())
	})

	It("should do nothing if there is no req in ctrlPort", func() {
		topPort.EXPECT().PeekIncoming().Return(nil)
		controlPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := tlbMW.lookup()
		madeProgress = ctrlMW.Tick() || madeProgress

		Expect(madeProgress).To(BeFalse())
	})

})
