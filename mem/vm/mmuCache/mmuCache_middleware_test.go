package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("MMUCacheMiddleware", func() {
	var (
		engine      timing.Engine
		sim         timing.Simulation
		comp        *Comp
		mw          *mmuCacheMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		spec := Definition.DefaultSpec
		spec.NumBlocks = 4
		spec.NumLevels = 2
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{
				LowModulePort: messaging.RemotePort("LowModule"),
				UpModulePort:  messaging.RemotePort("UpModule"),
			}).
			WithPorts(defaultPorts("MMUCache")).
			Build("MMUCache")

		topPort = comp.Ports.Top
		bottomPort = comp.Ports.Bottom
		controlPort = comp.Ports.Control
		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(bottomPort)
		(&noopConn{}).PlugIn(controlPort)

		mw = comp.Middlewares.Cache
	})

	It("should send full latency on miss", func() {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x2000,
			DeviceID: 3},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("UpModule"),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		topPort.Deliver(req)

		madeProgress := mw.lookup()

		Expect(madeProgress).To(BeTrue())

		sent, _ := bottomPort.RetrieveOutgoing()
		_, ok := sent.Payload.(vmprotocol.TranslationReq)
		sentReq := sent
		Expect(ok).To(BeTrue())
		Expect(sentReq.Payload.(vmprotocol.TranslationReq).TransLatency).To(Equal(uint64(200)))
		Expect(sentReq.Dst).To(Equal(messaging.RemotePort("LowModule")))
		Expect(sentReq.Src).To(Equal(bottomPort.AsRemote()))
		Expect(sentReq.Payload.(vmprotocol.TranslationReq).PID).To(Equal(vm.PID(1)))
		Expect(sentReq.Payload.(vmprotocol.TranslationReq).VAddr).To(Equal(uint64(0x2000)))
		Expect(sentReq.Payload.(vmprotocol.TranslationReq).DeviceID).To(Equal(uint64(3)))
		_, present0 := topPort.PeekIncoming()
		Expect(present0).To(BeFalse())
	})

	It("should reduce latency on upper-level hit", func() {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x3000,
			DeviceID: 2},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("UpModule"),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		// Compute seg and wayID for level 1
		spec := comp.Spec
		seg := segForLevelSpec(spec, 1, req.Payload.(vmprotocol.TranslationReq).VAddr)
		wayID := setIDForSegSpec(spec, seg)

		// Update the set in state
		next := &comp.State
		setUpdate(&next.Table[1], wayID, req.Payload.(vmprotocol.TranslationReq).PID, seg)

		topPort.Deliver(req)

		madeProgress := mw.lookup()

		Expect(madeProgress).To(BeTrue())

		sent, _ := bottomPort.RetrieveOutgoing()
		_, ok := sent.Payload.(vmprotocol.TranslationReq)
		sentReq := sent
		Expect(ok).To(BeTrue())
		Expect(sentReq.Payload.(vmprotocol.TranslationReq).TransLatency).To(Equal(uint64(100)))
	})

	It("should forward response and update cache", func() {
		page := vm.Page{
			PID:   1,
			VAddr: 0x4000,
			PAddr: 0x5000,
			Valid: true,
		}
		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: page,
		},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("LowModule"),
			Dst:          bottomPort.AsRemote(),
			RspTo:        sim.NewID(),
			TrafficClass: "vmprotocol.TranslationRsp"}

		bottomPort.Deliver(rsp)

		// Mark the response's request as outstanding, as a real forward would.
		comp.State.OutstandingBottomReqs[rsp.RspTo] = true

		madeProgress := mw.handleRsp(rsp)

		spec := comp.Spec
		next := &comp.State
		Expect(madeProgress).To(BeTrue())

		sent, _ := topPort.RetrieveOutgoing()
		_, ok := sent.Payload.(vmprotocol.TranslationRsp)
		sentRsp := sent
		Expect(ok).To(BeTrue())
		Expect(sentRsp.Dst).To(Equal(messaging.RemotePort("UpModule")))
		Expect(sentRsp.Src).To(Equal(topPort.AsRemote()))
		Expect(sentRsp.Payload.(vmprotocol.TranslationRsp).Page).To(Equal(page))

		for level := 0; level < spec.NumLevels; level++ {
			seg := segForLevelSpec(spec, level, page.VAddr)
			_, found := setLookup(&next.Table[level], page.PID, seg)
			Expect(found).To(BeTrue())
		}
	})

	It("drops the addressed page's segments while a sibling page survives", func() {
		pid := vm.PID(1)
		spec := comp.Spec

		// dropAddr and keepAddr share their upper-level (level-1) segment
		// but differ at level 0. A page-walk cache stores per-level VPN
		// segments, so invalidating dropAddr removes its level-0 segment
		// (and the shared upper-level segment), while keepAddr's distinct
		// level-0 segment must survive.
		dropAddr := uint64(0x6000)
		keepAddr := uint64(0x9000)
		Expect(segForLevelSpec(spec, 0, dropAddr)).
			ToNot(Equal(segForLevelSpec(spec, 0, keepAddr)))
		Expect(segForLevelSpec(spec, 1, dropAddr)).
			To(Equal(segForLevelSpec(spec, 1, keepAddr)))

		next := &comp.State
		next.CurrentState = mmuCacheStatePause

		for level := 0; level < spec.NumLevels; level++ {
			dropSeg := segForLevelSpec(spec, level, dropAddr)
			keepSeg := segForLevelSpec(spec, level, keepAddr)
			setUpdate(&next.Table[level], setIDForSegSpec(spec, dropSeg), pid, dropSeg)
			setVisit(&next.Table[level], setIDForSegSpec(spec, dropSeg))
			setUpdate(&next.Table[level], setIDForSegSpec(spec, keepSeg), pid, keepSeg)
			setVisit(&next.Table[level], setIDForSegSpec(spec, keepSeg))
		}

		ctrl := comp.Middlewares.Ctrl
		req := messaging.Msg{Payload: memcontrolprotocol.Req{
			Command:   memcontrolprotocol.CmdInvalidate,
			Addresses: []uint64{dropAddr},
		},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			Dst:          controlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		controlPort.Deliver(req)

		Expect(ctrl.handleIncomingCommands()).To(BeTrue())

		// The dropped address's segment is gone at every level it cached.
		for level := 0; level < spec.NumLevels; level++ {
			dropSeg := segForLevelSpec(spec, level, dropAddr)
			_, dropFound := setLookup(&next.Table[level], pid, dropSeg)
			Expect(dropFound).To(BeFalse())
		}
		// keepAddr's distinct level-0 segment survives.
		keepSeg0 := segForLevelSpec(spec, 0, keepAddr)
		_, keepFound := setLookup(&next.Table[0], pid, keepSeg0)
		Expect(keepFound).To(BeTrue())

		sentRspValue, _ := controlPort.RetrieveOutgoing()

		sentRsp := sentRspValue
		Expect(sentRsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(sentRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
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
