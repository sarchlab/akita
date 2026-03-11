package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("TLB CtrlMiddleware", func() {

	var (
		mockCtrl    *gomock.Controller
		engine      *MockEngine
		comp        *Comp
		ctrlMW      *ctrlMiddleware
		controlPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("ControlPort")).
			AnyTimes()
		controlPort.EXPECT().
			Name().
			Return("ControlPort").
			AnyTimes()
		controlPort.EXPECT().
			SetComponent(gomock.Any()).
			AnyTimes()

		topPort := NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()
		topPort.EXPECT().
			Name().
			Return("TopPort").
			AnyTimes()
		topPort.EXPECT().
			SetComponent(gomock.Any()).
			AnyTimes()

		bottomPort := NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()
		bottomPort.EXPECT().
			Name().
			Return("BottomPort").
			AnyTimes()
		bottomPort.EXPECT().
			SetComponent(gomock.Any()).
			AnyTimes()

		comp = MakeBuilder().
			WithEngine(engine).
			WithTranslationProviderMapperType("single").
			WithTranslationProviders(sim.RemotePort("RemotePort")).
			WithTopPort(topPort).
			WithBottomPort(bottomPort).
			WithControlPort(controlPort).
			Build("TLB")

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
