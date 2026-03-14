package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MMUCacheMiddleware", func() {
	var (
		mockCtrl    *gomock.Controller
		comp        *modeling.Component[Spec, State]
		mw          *mmuCacheMiddleware
		topPort     *MockPort
		bottomPort  *MockPort
		controlPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().AsRemote().Return(sim.RemotePort("TopPort")).AnyTimes()
		topPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().AsRemote().Return(sim.RemotePort("BottomPort")).AnyTimes()
		bottomPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().AsRemote().Return(sim.RemotePort("ControlPort")).AnyTimes()
		controlPort.EXPECT().Name().Return("ControlPort").AnyTimes()
		controlPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		spec := Spec{
			NumBlocks:       4,
			NumLevels:       2,
			PageSize:        4096,
			Log2PageSize:    12,
			NumReqPerCycle:  4,
			LatencyPerLevel: 100,
			LowModulePort:   sim.RemotePort("LowModule"),
			UpModulePort:    sim.RemotePort("UpModule"),
		}

		initialState := State{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		}

		comp = modeling.NewBuilder[Spec, State]().
			WithSpec(spec).
			Build("MMUCache")
		comp.SetState(initialState)

		comp.AddPort("Top", topPort)
		comp.AddPort("Bottom", bottomPort)
		comp.AddPort("Control", controlPort)

		mw = &mmuCacheMiddleware{comp: comp}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send full latency on miss", func() {
		req := &vm.TranslationReq{}
		req.ID = sim.GetIDGenerator().Generate()
		req.PID = 1
		req.VAddr = 0x2000
		req.DeviceID = 3
		req.TrafficClass = "vm.TranslationReq"

		topPort.EXPECT().PeekIncoming().Return(req)
		bottomPort.EXPECT().CanSend().Return(true).AnyTimes()
		bottomPort.EXPECT().Send(gomock.Any()).Do(func(sent sim.Msg) {
			req := sent.(*vm.TranslationReq)
			Expect(req.TransLatency).To(Equal(uint64(200)))
			Expect(req.Dst).To(Equal(sim.RemotePort("LowModule")))
			Expect(req.Src).To(Equal(sim.RemotePort("BottomPort")))
			Expect(req.PID).To(Equal(vm.PID(1)))
			Expect(req.VAddr).To(Equal(uint64(0x2000)))
			Expect(req.DeviceID).To(Equal(uint64(3)))
		}).Return(nil)
		topPort.EXPECT().RetrieveIncoming().Return(req)

		madeProgress := mw.lookup()

		Expect(madeProgress).To(BeTrue())
	})

	It("should reduce latency on upper-level hit", func() {
		req := &vm.TranslationReq{}
		req.ID = sim.GetIDGenerator().Generate()
		req.PID = 1
		req.VAddr = 0x3000
		req.DeviceID = 2
		req.TrafficClass = "vm.TranslationReq"

		// Compute seg and wayID for level 1
		spec := comp.GetSpec()
		seg := segForLevelSpec(spec, 1, req.VAddr)
		wayID := setIDForSegSpec(spec, seg)

		// Update the set in state
		next := comp.GetNextState()
		setUpdate(&next.Table[1], wayID, req.PID, seg)

		topPort.EXPECT().PeekIncoming().Return(req)
		bottomPort.EXPECT().CanSend().Return(true).AnyTimes()
		bottomPort.EXPECT().Send(gomock.Any()).Do(func(sent sim.Msg) {
			req := sent.(*vm.TranslationReq)
			Expect(req.TransLatency).To(Equal(uint64(100)))
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
		rsp := &vm.TranslationRsp{
			Page: page,
		}
		rsp.ID = sim.GetIDGenerator().Generate()
		rsp.RspTo = sim.GetIDGenerator().Generate()
		rsp.TrafficClass = "vm.TranslationRsp"

		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).Do(func(sent sim.Msg) {
			rsp := sent.(*vm.TranslationRsp)
			Expect(rsp.Dst).To(Equal(sim.RemotePort("UpModule")))
			Expect(rsp.Src).To(Equal(sim.RemotePort("TopPort")))
			Expect(rsp.Page).To(Equal(page))
		}).Return(nil)
		bottomPort.EXPECT().RetrieveIncoming()

		madeProgress := mw.handleRsp(rsp)

		spec := comp.GetSpec()
		next := comp.GetNextState()
		Expect(madeProgress).To(BeTrue())
		for level := 0; level < spec.NumLevels; level++ {
			seg := segForLevelSpec(spec, level, page.VAddr)
			_, found := setLookup(&next.Table[level], page.PID, seg)
			Expect(found).To(BeTrue())
		}
	})

	It("should flush and reset cache", func() {
		pid := vm.PID(1)
		vAddr := uint64(0x6000)
		spec := comp.GetSpec()
		seg := segForLevelSpec(spec, 0, vAddr)
		wayID := setIDForSegSpec(spec, seg)

		next := comp.GetNextState()
		setUpdate(&next.Table[0], wayID, pid, seg)

		// Set up flush state
		next.InflightFlushReqActive = true
		next.InflightFlushReqID = sim.GetIDGenerator().Generate()
		next.InflightFlushReqSrc = sim.RemotePort("Requester")
		next.CurrentState = mmuCacheStateFlush

		controlPort.EXPECT().Send(gomock.Any()).Do(func(sent sim.Msg) {
			rsp := sent.(*tlb.FlushRsp)
			Expect(rsp.Dst).To(Equal(sim.RemotePort("Requester")))
			Expect(rsp.Src).To(Equal(sim.RemotePort("ControlPort")))
		}).Return(nil)

		madeProgress := mw.processMMUCacheFlush()

		next = comp.GetNextState()
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStatePause))
		Expect(next.InflightFlushReqActive).To(BeFalse())
		_, found := setLookup(&next.Table[0], pid, seg)
		Expect(found).To(BeFalse())
	})
})

func segForLevelSpec(spec Spec, level int, vAddr uint64) uint64 {
	vpn := vAddr >> spec.Log2PageSize
	levelWidth := (64 - spec.Log2PageSize) / uint64(spec.NumLevels)
	return (vpn >> (uint64(level) * levelWidth)) & ((1 << levelWidth) - 1)
}

func setIDForSegSpec(spec Spec, seg uint64) int {
	return int(seg % uint64(spec.NumBlocks))
}
