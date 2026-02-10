package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MMUCacheCtrlMiddleware", func() {
	var (
		mockCtrl    *gomock.Controller
		engine      sim.Engine
		cache       *Comp
		ctrl        *ctrlMiddleware
		topPort     *MockPort
		bottomPort  *MockPort
		controlPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()

		topPort = NewMockPort(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().AsRemote().Return(sim.RemotePort("ControlPort")).AnyTimes()
		controlPort.EXPECT().Name().Return("ControlPort").AnyTimes()

		cache = MakeBuilder().
			WithEngine(engine).
			Build("MMUCache")
		cache.topPort = topPort
		cache.bottomPort = bottomPort
		cache.controlPort = controlPort
		cache.state = mmuCacheStatePause

		ctrl = &ctrlMiddleware{Comp: cache}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing when no control message", func() {
		controlPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := ctrl.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should restart and drain ports", func() {
		req := RestartReqBuilder{}.
			WithSrc(sim.RemotePort("Requester")).
			Build()

		topMsg := vm.TranslationReqBuilder{}.
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(1).
			Build()
		bottomMsg := vm.TranslationRspBuilder{}.
			WithRspTo(sim.GetIDGenerator().Generate()).
			WithPage(vm.Page{}).
			Build()

		controlPort.EXPECT().Send(gomock.Any()).Do(func(rsp *RestartRsp) {
			Expect(rsp.Dst).To(Equal(sim.RemotePort("Requester")))
			Expect(rsp.Src).To(Equal(sim.RemotePort("ControlPort")))
		}).Return(nil)
		controlPort.EXPECT().RetrieveIncoming()

		topPort.EXPECT().PeekIncoming().Return(topMsg)
		topPort.EXPECT().RetrieveIncoming()
		topPort.EXPECT().PeekIncoming().Return(nil)

		bottomPort.EXPECT().PeekIncoming().Return(bottomMsg)
		bottomPort.EXPECT().RetrieveIncoming()
		bottomPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := ctrl.handleMMUCacheRestart(req)

		Expect(madeProgress).To(BeTrue())
		Expect(cache.state).To(Equal(mmuCacheStateEnable))
	})

	It("should accept flush request in enable state", func() {
		cache.state = mmuCacheStateEnable
		req := FlushReqBuilder{}.
			WithSrc(sim.RemotePort("Requester")).
			Build()

		controlPort.EXPECT().RetrieveIncoming()

		madeProgress := ctrl.handleMMUCacheFlush(req)

		Expect(madeProgress).To(BeTrue())
		Expect(cache.inflightFlushReq).To(Equal(req))
		Expect(cache.state).To(Equal(mmuCacheStateFlush))
	})

	It("should handle control pause", func() {
		cache.state = mmuCacheStateEnable
		msg := mem.ControlMsgBuilder{}.
			WithDst(sim.RemotePort("ControlPort")).
			WithCtrlInfo(false, false, false, true, false).
			Build()

		controlPort.EXPECT().PeekIncoming().Return(msg)
		controlPort.EXPECT().PeekIncoming().Return(msg)
		controlPort.EXPECT().RetrieveIncoming().Return(msg)

		madeProgress := ctrl.handleIncomingCommands()

		Expect(madeProgress).To(BeTrue())
		Expect(cache.state).To(Equal(mmuCacheStatePause))
	})
})
