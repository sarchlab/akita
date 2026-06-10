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

var _ = Describe("MMUCacheCtrlMiddleware", func() {
	var (
		engine      timing.Engine
		comp        *Comp
		ctrl        *ctrlMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		spec := DefaultSpec()
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		reg := modeling.NewStandaloneRegistrar(engine)
		comp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("MMUCache")
		comp.State.CurrentState = mmuCacheStatePause

		assignDefaultPorts(reg, comp)

		topPort = comp.GetPortByName("Top")
		bottomPort = comp.GetPortByName("Bottom")
		controlPort = comp.GetPortByName("Control")
		(&noopConn{}).PlugIn(topPort)
		(&noopConn{}).PlugIn(bottomPort)
		(&noopConn{}).PlugIn(controlPort)

		ctrl = &ctrlMiddleware{comp: comp}
	})

	It("should do nothing when no control message", func() {
		madeProgress := ctrl.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should restart and drain ports", func() {
		req := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.TrafficClass = "memcontrolprotocol.Req"

		topMsg := vmprotocol.TranslationReq{}
		topMsg.ID = timing.GetIDGenerator().Generate()
		topMsg.Src = messaging.RemotePort("Requester")
		topMsg.Dst = topPort.AsRemote()
		topMsg.PID = 1
		topMsg.VAddr = 0x1000
		topMsg.DeviceID = 1
		topMsg.TrafficClass = "vmprotocol.TranslationReq"
		topPort.Deliver(topMsg)

		bottomMsg := vmprotocol.TranslationRsp{
			Page: vm.Page{},
		}
		bottomMsg.ID = timing.GetIDGenerator().Generate()
		bottomMsg.Src = messaging.RemotePort("LowModule")
		bottomMsg.Dst = bottomPort.AsRemote()
		bottomMsg.RspTo = timing.GetIDGenerator().Generate()
		bottomMsg.TrafficClass = "vmprotocol.TranslationRsp"
		bottomPort.Deliver(bottomMsg)

		madeProgress := ctrl.handleReset(req)

		next := &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStateEnable))
		Expect(topPort.PeekIncoming()).To(BeNil())
		Expect(bottomPort.PeekIncoming()).To(BeNil())

		rsp := controlPort.RetrieveOutgoing()
		ctrlRsp, ok := rsp.(memcontrolprotocol.Rsp)
		Expect(ok).To(BeTrue())
		Expect(ctrlRsp.Command).To(Equal(memcontrolprotocol.CmdReset))
		Expect(ctrlRsp.Success).To(BeTrue())
		Expect(ctrlRsp.Dst).To(Equal(messaging.RemotePort("Requester")))
		Expect(ctrlRsp.Src).To(Equal(controlPort.AsRemote()))
	})

	It("should reject Flush as unsupported", func() {
		req := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.Dst = controlPort.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
		controlPort.Deliver(req)

		Expect(ctrl.handleIncomingCommands()).To(BeTrue())

		rsp := controlPort.RetrieveOutgoing().(memcontrolprotocol.Rsp)
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdFlush))
		Expect(rsp.Success).To(BeFalse())
		Expect(rsp.Error).To(Equal(memcontrolprotocol.ErrUnsupported))
	})

	It("should invalidate cached segments when paused", func() {
		spec := comp.Spec()
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

		req := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.Dst = controlPort.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
		controlPort.Deliver(req)

		Expect(ctrl.handleIncomingCommands()).To(BeTrue())

		_, found = setLookup(&next.Table[0], pid, seg)
		Expect(found).To(BeFalse())

		rsp := controlPort.RetrieveOutgoing().(memcontrolprotocol.Rsp)
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Success).To(BeTrue())
	})

	It("should reject Invalidate while enabled", func() {
		next := &comp.State
		next.CurrentState = mmuCacheStateEnable

		req := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.Dst = controlPort.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
		controlPort.Deliver(req)

		Expect(ctrl.handleIncomingCommands()).To(BeTrue())

		rsp := controlPort.RetrieveOutgoing().(memcontrolprotocol.Rsp)
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Success).To(BeFalse())
		Expect(rsp.Error).To(Equal(memcontrolprotocol.ErrMustBePausedOrDrained))
	})

	It("invalidates only entries matching the PID filter", func() {
		// A two-way single-level cache so two PIDs can coexist in
		// distinct ways and the filter's selectivity is observable.
		spec := DefaultSpec()
		spec.NumBlocks = 2
		spec.NumLevels = 1
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		reg2 := modeling.NewStandaloneRegistrar(engine)
		comp2 := MakeBuilder().
			WithRegistrar(reg2).
			WithSpec(spec).
			Build("MMUCache2")
		assignDefaultPorts(reg2, comp2)
		control2 := comp2.GetPortByName("Control")
		for _, name := range []string{"Top", "Bottom", "Control"} {
			(&noopConn{}).PlugIn(comp2.GetPortByName(name))
		}
		ctrl2 := &ctrlMiddleware{comp: comp2}

		next := &comp2.State
		next.CurrentState = mmuCacheStatePause

		segA := uint64(0xa)
		segB := uint64(0xb)
		setUpdate(&next.Table[0], 0, vm.PID(1), segA)
		setVisit(&next.Table[0], 0)
		setUpdate(&next.Table[0], 1, vm.PID(2), segB)
		setVisit(&next.Table[0], 1)

		req := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, PID: 1}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.Dst = control2.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
		control2.Deliver(req)

		Expect(ctrl2.handleIncomingCommands()).To(BeTrue())

		// Only the PID-1 entry is dropped; the PID-2 entry survives.
		_, foundA := setLookup(&next.Table[0], vm.PID(1), segA)
		Expect(foundA).To(BeFalse())
		_, foundB := setLookup(&next.Table[0], vm.PID(2), segB)
		Expect(foundB).To(BeTrue())

		rsp := control2.RetrieveOutgoing().(memcontrolprotocol.Rsp)
		Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
		Expect(rsp.Success).To(BeTrue())
	})

	It("should handle control pause", func() {
		spec := comp.Spec()
		comp.State = State{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		}

		msg := memcontrolprotocol.Req{
			Command: memcontrolprotocol.CmdPause,
		}
		msg.ID = timing.GetIDGenerator().Generate()
		msg.Src = messaging.RemotePort("Requester")
		msg.Dst = controlPort.AsRemote()
		msg.TrafficBytes = 4
		msg.TrafficClass = "memcontrolprotocol.Req"
		controlPort.Deliver(msg)

		madeProgress := ctrl.handleIncomingCommands()

		next := &comp.State
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStatePause))
	})
})
