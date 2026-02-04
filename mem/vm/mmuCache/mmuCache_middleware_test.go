package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MMUCacheMiddleware", func() {
	var (
		mockCtrl    *gomock.Controller
		engine      sim.Engine
		cache       *Comp
		mw          *mmuCacheMiddleware
		topPort     *MockPort
		bottomPort  *MockPort
		controlPort *MockPort
		lowModule   *MockPort
		upModule    *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().Return(sim.RemotePort("TopPort")).AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().AsRemote().Return(sim.RemotePort("BottomPort")).AnyTimes()
		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().AsRemote().Return(sim.RemotePort("ControlPort")).AnyTimes()
		controlPort.EXPECT().Name().Return("ControlPort").AnyTimes()
		lowModule = NewMockPort(mockCtrl)
		lowModule.EXPECT().AsRemote().Return(sim.RemotePort("LowModule")).AnyTimes()
		upModule = NewMockPort(mockCtrl)
		upModule.EXPECT().AsRemote().Return(sim.RemotePort("UpModule")).AnyTimes()

		cache = MakeBuilder().
			WithEngine(engine).
			WithNumLevels(2).
			WithNumBlocks(4).
			WithLatencyPerLevel(100).
			Build("MMUCache")
		cache.numBlocks = 4
		cache.reset()
		cache.topPort = topPort
		cache.bottomPort = bottomPort
		cache.controlPort = controlPort
		cache.LowModule = lowModule
		cache.UpModule = upModule
		cache.state = mmuCacheStateEnable

		mw = &mmuCacheMiddleware{Comp: cache}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send full latency on miss", func() {
		req := vm.TranslationReqBuilder{}.
			WithPID(1).
			WithVAddr(0x2000).
			WithDeviceID(3).
			Build()

		topPort.EXPECT().PeekIncoming().Return(req)
		bottomPort.EXPECT().CanSend().Return(true).AnyTimes()
		bottomPort.EXPECT().Send(gomock.Any()).Do(func(sent *vm.TranslationReq) {
			Expect(sent.TransLatency).To(Equal(uint64(200)))
			Expect(sent.Dst).To(Equal(sim.RemotePort("LowModule")))
			Expect(sent.Src).To(Equal(sim.RemotePort("BottomPort")))
			Expect(sent.PID).To(Equal(vm.PID(1)))
			Expect(sent.VAddr).To(Equal(uint64(0x2000)))
			Expect(sent.DeviceID).To(Equal(uint64(3)))
		}).Return(nil)
		topPort.EXPECT().RetrieveIncoming().Return(req)

		madeProgress := mw.lookup()

		Expect(madeProgress).To(BeTrue())
	})

	It("should reduce latency on upper-level hit", func() {
		req := vm.TranslationReqBuilder{}.
			WithPID(1).
			WithVAddr(0x3000).
			WithDeviceID(2).
			Build()

		seg := segForLevel(cache, 1, req.VAddr)
		wayID := setIDForSeg(cache, seg)
		cache.table[1].Update(wayID, req.PID, seg)

		topPort.EXPECT().PeekIncoming().Return(req)
		bottomPort.EXPECT().CanSend().Return(true).AnyTimes()
		bottomPort.EXPECT().Send(gomock.Any()).Do(func(sent *vm.TranslationReq) {
			Expect(sent.TransLatency).To(Equal(uint64(100)))
		}).Return(nil)
		topPort.EXPECT().RetrieveIncoming().Return(req)

		madeProgress := mw.lookup()

		Expect(madeProgress).To(BeTrue())
	})

	It("should forward response and update cache", func() {
		page := vm.Page{
			PID:   1,
			VAddr: 0x4000,
			PAddr: 0x5000,
			Valid: true,
		}
		rsp := vm.TranslationRspBuilder{}.
			WithRspTo(sim.GetIDGenerator().Generate()).
			WithPage(page).
			Build()

		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).Do(func(sent *vm.TranslationRsp) {
			Expect(sent.Dst).To(Equal(sim.RemotePort("UpModule")))
			Expect(sent.Src).To(Equal(sim.RemotePort("TopPort")))
			Expect(sent.Page).To(Equal(page))
		}).Return(nil)
		bottomPort.EXPECT().RetrieveIncoming()

		madeProgress := mw.handleRsp(rsp)

		Expect(madeProgress).To(BeTrue())
		for level := 0; level < cache.numLevels; level++ {
			seg := segForLevel(cache, level, page.VAddr)
			_, found := cache.table[level].Lookup(page.PID, seg)
			Expect(found).To(BeTrue())
		}
	})

	It("should flush and reset cache", func() {
		pid := vm.PID(1)
		vAddr := uint64(0x6000)
		seg := segForLevel(cache, 0, vAddr)
		wayID := setIDForSeg(cache, seg)
		cache.table[0].Update(wayID, pid, seg)

		cache.inflightFlushReq = FlushReqBuilder{}.
			WithSrc(sim.RemotePort("Requester")).
			Build()
		cache.state = mmuCacheStateFlush

		controlPort.EXPECT().Send(gomock.Any()).Do(func(rsp *FlushRsp) {
			Expect(rsp.Dst).To(Equal(sim.RemotePort("Requester")))
			Expect(rsp.Src).To(Equal(sim.RemotePort("ControlPort")))
		}).Return(nil)

		madeProgress := mw.processmmuCacheFlush()

		Expect(madeProgress).To(BeTrue())
		Expect(cache.state).To(Equal(mmuCacheStatePause))
		Expect(cache.inflightFlushReq).To(BeNil())
		_, found := cache.table[0].Lookup(pid, seg)
		Expect(found).To(BeFalse())
	})
})

func segForLevel(cache *Comp, level int, vAddr uint64) uint64 {
	vpn := vAddr >> cache.log2PageSize
	levelWidth := (64 - cache.log2PageSize) / uint64(cache.numLevels)
	return (vpn >> (uint64(level) * levelWidth)) & ((1 << levelWidth) - 1)
}

func setIDForSeg(cache *Comp, seg uint64) int {
	return int(seg % uint64(cache.numBlocks))
}
