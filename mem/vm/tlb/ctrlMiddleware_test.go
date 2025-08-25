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

		comp = MakeBuilder().
			WithEngine(engine).
			WithTranslationProviderMapperType("single").
			WithTranslationProviders("RemotePort").
			Build("TLB")
		comp.topPort = topPort
		comp.bottomPort = bottomPort
		comp.controlPort = controlPort
		comp.sets = []internal.Set{set}

		ctrlMW = comp.Middlewares()[0].(*ctrlMiddleware)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no req in ctrlPort", func() {
		controlPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := ctrlMW.Tick()

		Expect(madeProgress).To(BeFalse())
	})

})
