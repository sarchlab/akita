package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("MMUCacheMiddleware", func() {
	var (
		engine      timing.Engine
		comp        *Comp
		mw          *mmuCacheMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.NumBlocks = 4
		spec.NumLevels = 2
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{
				LowModulePort: messaging.RemotePort("LowModule"),
				UpModulePort:  messaging.RemotePort("UpModule"),
			}).
			Build("MMUCache")

		topPort = comp.GetPortByName("Top")
		bottomPort = comp.GetPortByName("Bottom")
		controlPort = comp.GetPortByName("Control")
		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(bottomPort)
		(&noopConn{}).PlugIn(controlPort)

		mw = &mmuCacheMiddleware{comp: comp}
	})

	It("should send full latency on miss", func() {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("UpModule")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = 0x2000
		req.DeviceID = 3
		req.TrafficClass = "vm.TranslationReq"
		topPort.Deliver(req)

		madeProgress := mw.lookup()

		Expect(madeProgress).To(BeTrue())

		sent := bottomPort.RetrieveOutgoing()
		sentReq, ok := sent.(vm.TranslationReq)
		Expect(ok).To(BeTrue())
		Expect(sentReq.TransLatency).To(Equal(uint64(200)))
		Expect(sentReq.Dst).To(Equal(messaging.RemotePort("LowModule")))
		Expect(sentReq.Src).To(Equal(bottomPort.AsRemote()))
		Expect(sentReq.PID).To(Equal(vm.PID(1)))
		Expect(sentReq.VAddr).To(Equal(uint64(0x2000)))
		Expect(sentReq.DeviceID).To(Equal(uint64(3)))
		Expect(topPort.PeekIncoming()).To(BeNil())
	})

	It("should reduce latency on upper-level hit", func() {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("UpModule")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = 0x3000
		req.DeviceID = 2
		req.TrafficClass = "vm.TranslationReq"

		// Compute seg and wayID for level 1
		spec := comp.Spec()
		seg := segForLevelSpec(spec, 1, req.VAddr)
		wayID := setIDForSegSpec(spec, seg)

		// Update the set in state
		next := &comp.State
		setUpdate(&next.Table[1], wayID, req.PID, seg)

		topPort.Deliver(req)

		madeProgress := mw.lookup()

		Expect(madeProgress).To(BeTrue())

		sent := bottomPort.RetrieveOutgoing()
		sentReq, ok := sent.(vm.TranslationReq)
		Expect(ok).To(BeTrue())
		Expect(sentReq.TransLatency).To(Equal(uint64(100)))
	})

	It("should forward response and update cache", func() {
		page := vm.Page{
			PID:   1,
			VAddr: 0x4000,
			PAddr: 0x5000,
			Valid: true,
		}
		rsp := vm.TranslationRsp{
			Page: page,
		}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = messaging.RemotePort("LowModule")
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = timing.GetIDGenerator().Generate()
		rsp.TrafficClass = "vm.TranslationRsp"
		bottomPort.Deliver(rsp)

		madeProgress := mw.handleRsp(rsp)

		spec := comp.Spec()
		next := &comp.State
		Expect(madeProgress).To(BeTrue())

		sent := topPort.RetrieveOutgoing()
		sentRsp, ok := sent.(vm.TranslationRsp)
		Expect(ok).To(BeTrue())
		Expect(sentRsp.Dst).To(Equal(messaging.RemotePort("UpModule")))
		Expect(sentRsp.Src).To(Equal(topPort.AsRemote()))
		Expect(sentRsp.Page).To(Equal(page))

		for level := 0; level < spec.NumLevels; level++ {
			seg := segForLevelSpec(spec, level, page.VAddr)
			_, found := setLookup(&next.Table[level], page.PID, seg)
			Expect(found).To(BeTrue())
		}
	})

	It("should flush and reset cache", func() {
		pid := vm.PID(1)
		vAddr := uint64(0x6000)
		spec := comp.Spec()
		seg := segForLevelSpec(spec, 0, vAddr)
		wayID := setIDForSegSpec(spec, seg)

		next := &comp.State
		setUpdate(&next.Table[0], wayID, pid, seg)

		// Set up flush state
		next.InflightFlushReqActive = true
		next.InflightFlushReqID = timing.GetIDGenerator().Generate()
		next.InflightFlushReqSrc = messaging.RemotePort("Requester")
		next.CurrentState = mmuCacheStateFlush

		madeProgress := mw.processMMUCacheFlush()

		next = &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStatePause))
		Expect(next.InflightFlushReqActive).To(BeFalse())
		_, found := setLookup(&next.Table[0], pid, seg)
		Expect(found).To(BeFalse())

		sent := controlPort.RetrieveOutgoing()
		sentRsp, ok := sent.(mem.ControlRsp)
		Expect(ok).To(BeTrue())
		Expect(sentRsp.Command).To(Equal(mem.CmdFlush))
		Expect(sentRsp.Success).To(BeTrue())
		Expect(sentRsp.Dst).To(Equal(messaging.RemotePort("Requester")))
		Expect(sentRsp.Src).To(Equal(controlPort.AsRemote()))
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
