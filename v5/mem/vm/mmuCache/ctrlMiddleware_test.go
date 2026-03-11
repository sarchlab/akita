package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
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
			WithTopPort(sim.NewPort(nil, 4800, 4800, "MMUCache.TopPort")).
			WithBottomPort(sim.NewPort(nil, 4800, 4800, "MMUCache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 1, 1, "MMUCache.ControlPort")).
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
		req := &RestartReq{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = sim.RemotePort("Requester")
		req.TrafficClass = "mmuCache.RestartReq"

		topMsg := &vm.TranslationReq{}
		topMsg.ID = sim.GetIDGenerator().Generate()
		topMsg.PID = 1
		topMsg.VAddr = 0x1000
		topMsg.DeviceID = 1
		topMsg.TrafficClass = "vm.TranslationReq"
		bottomMsg := &vm.TranslationRsp{
			Page: vm.Page{},
		}
		bottomMsg.ID = sim.GetIDGenerator().Generate()
		bottomMsg.RspTo = sim.GetIDGenerator().Generate()
		bottomMsg.TrafficClass = "vm.TranslationRsp"

		controlPort.EXPECT().Send(gomock.Any()).Do(func(sent sim.Msg) {
			rsp := sent.(*RestartRsp)
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
		req := &FlushReq{}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = sim.RemotePort("Requester")
		req.TrafficClass = "mmuCache.FlushReq"
		controlPort.EXPECT().RetrieveIncoming()

		madeProgress := ctrl.handleMMUCacheFlush(req)

		Expect(madeProgress).To(BeTrue())
		Expect(cache.inflightFlushReq).To(Equal(req))
		Expect(cache.state).To(Equal(mmuCacheStateFlush))
	})

	It("should handle control pause", func() {
		cache.state = mmuCacheStateEnable
		msg := &mem.ControlMsg{
			Pause: true,
		}
		msg.ID = sim.GetIDGenerator().Generate()
		msg.Dst = sim.RemotePort("ControlPort")
		msg.TrafficBytes = 4
		msg.TrafficClass = "mem.ControlMsg"

		controlPort.EXPECT().PeekIncoming().Return(msg)
		controlPort.EXPECT().PeekIncoming().Return(msg)
		controlPort.EXPECT().RetrieveIncoming().Return(msg)

		madeProgress := ctrl.handleIncomingCommands()

		Expect(madeProgress).To(BeTrue())
		Expect(cache.state).To(Equal(mmuCacheStatePause))
	})
})
