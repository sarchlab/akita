package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("MMUCacheCtrlMiddleware", func() {
	var (
		engine      timing.Engine
		sim         timing.Simulation
		comp        *Comp
		ctrl        *ctrlMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)

		spec := Definition.DefaultSpec
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithPorts(defaultPorts("MMUCache")).
			Build("MMUCache")
		comp.State.CurrentState = mmuCacheStatePause

		topPort = comp.Ports.Top
		bottomPort = comp.Ports.Bottom
		controlPort = comp.Ports.Control
		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(bottomPort)
		(&noopConn{}).PlugIn(controlPort)

		ctrl = comp.Middlewares.Ctrl
	})

	It("should do nothing when no control message", func() {
		madeProgress := ctrl.Handle(modelingtest.TickEvent(comp))

		Expect(madeProgress).To(BeFalse())
	})

	It("should restart and drain ports", func() {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			TrafficClass: "memcontrolprotocol.Req"}

		topMsg := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		topPort.Deliver(topMsg)

		bottomMsg := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{},
		},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("LowModule"),
			Dst:          bottomPort.AsRemote(),
			RspTo:        sim.NewID(),
			TrafficClass: "vmprotocol.TranslationRsp"}

		bottomPort.Deliver(bottomMsg)

		madeProgress := ctrl.handleReset(req)

		next := &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStateEnable))
		_, present0 := topPort.PeekIncoming()
		Expect(present0).To(BeFalse())
		_, present1 := bottomPort.PeekIncoming()
		Expect(present1).To(BeFalse())
		rsp, _ := controlPort.RetrieveOutgoing()
		_, ok := rsp.Payload.(memcontrolprotocol.Rsp)
		ctrlRsp := rsp
		Expect(ok).To(BeTrue())
		Expect(ctrlRsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
		Expect(ctrlRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		Expect(ctrlRsp.Dst).To(Equal(messaging.RemotePort("Requester")))
		Expect(ctrlRsp.Src).To(Equal(controlPort.AsRemote()))
	})

	It("should reject Flush as unsupported", func() {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			Dst:          controlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		controlPort.Deliver(req)

		Expect(ctrl.handleIncomingCommands()).To(BeTrue())

		rspValue, _ := controlPort.RetrieveOutgoing()

		rsp := rspValue
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdFlush))
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeFalse())
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Error).To(Equal(memcontrolprotocol.ErrUnsupported))
	})

	It("should invalidate cached segments when paused", func() {
		spec := comp.Spec
		next := &comp.State
		next.CurrentState = mmuCacheStatePause

		pid := vm.PID(1)
		vAddr := uint64(0x6000)
		seg := segForLevelSpec(spec, 0, vAddr)
		wayID := setIDForSegSpec(spec, seg)
		setUpdate(&next.Table[0], wayID, pid, seg)
		setVisit(&next.Table[0], wayID)

		_, found := setLookup(&next.Table[0], pid, seg)
		Expect(found).To(BeTrue())

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			Dst:          controlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		controlPort.Deliver(req)

		Expect(ctrl.handleIncomingCommands()).To(BeTrue())

		_, found = setLookup(&next.Table[0], pid, seg)
		Expect(found).To(BeFalse())

		rspValue, _ := controlPort.RetrieveOutgoing()

		rsp := rspValue
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
	})

	It("should reject Invalidate while enabled", func() {
		next := &comp.State
		next.CurrentState = mmuCacheStateEnable

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			Dst:          controlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		controlPort.Deliver(req)

		Expect(ctrl.handleIncomingCommands()).To(BeTrue())

		rspValue, _ := controlPort.RetrieveOutgoing()

		rsp := rspValue
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeFalse())
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Error).To(Equal(memcontrolprotocol.ErrMustBePausedOrDrained))
	})

	It("invalidates only entries matching the PID filter", func() {
		// A two-way single-level cache so two PIDs can coexist in
		// distinct ways and the filter's selectivity is observable.
		spec := Definition.DefaultSpec
		spec.NumBlocks = 2
		spec.NumLevels = 1
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		reg2 := sim
		comp2 := Definition.Builder().
			WithSimulation(reg2).
			WithSpec(spec).
			WithPorts(defaultPorts("MMUCache2")).
			Build("MMUCache2")
		control2 := comp2.Ports.Control
		for _, p := range []messaging.Port{
			comp2.Ports.Top, comp2.Ports.Bottom, comp2.Ports.Control,
		} {
			(&noopConn{}).PlugIn(p)
		}
		ctrl2 := comp2.Middlewares.Ctrl

		next := &comp2.State
		next.CurrentState = mmuCacheStatePause

		segA := uint64(0xa)
		segB := uint64(0xb)
		setUpdate(&next.Table[0], 0, vm.PID(1), segA)
		setVisit(&next.Table[0], 0)
		setUpdate(&next.Table[0], 1, vm.PID(2), segB)
		setVisit(&next.Table[0], 1)

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, PID: 1},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			Dst:          control2.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		control2.Deliver(req)

		Expect(ctrl2.handleIncomingCommands()).To(BeTrue())

		// Only the PID-1 entry is dropped; the PID-2 entry survives.
		_, foundA := setLookup(&next.Table[0], vm.PID(1), segA)
		Expect(foundA).To(BeFalse())
		_, foundB := setLookup(&next.Table[0], vm.PID(2), segB)
		Expect(foundB).To(BeTrue())

		rspValue, _ := control2.RetrieveOutgoing()

		rsp := rspValue
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
	})

	It("should handle control pause", func() {
		spec := comp.Spec
		comp.State = state{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		}

		msg := messaging.Msg{Payload: memcontrolprotocol.Req{
			Command: memcontrolprotocol.CmdPause,
		},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			Dst:          controlPort.AsRemote(),
			TrafficBytes: 4,
			TrafficClass: "memcontrolprotocol.Req"}

		controlPort.Deliver(msg)

		madeProgress := ctrl.handleIncomingCommands()

		next := &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStatePause))
	})
})
