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

// This file holds Layer-2 control-behavior tests for the mmuCache: it asserts
// the actual behavior the universal verbs promise (Drain quiescence, Pause
// freeze, Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The mmuCache differs from the idealmemcontroller reference in two ways:
//   - Its control state is a STRING field comp.State.CurrentState with values
//     mmuCacheStateEnable / mmuCacheStatePause / mmuCacheStateDrain (not the
//     control.State enum).
//   - It is a stateless forwarder with no MSHR: a Top lookup is forwarded down
//     the Bottom port (an empty cache always misses), and it does not emit a
//     Top response on its own. So "completion" for the Drain test is the
//     request appearing on the Bottom port's outgoing queue, and Drain
//     quiesces when both Top and Bottom incoming queues are empty.
var _ = Describe("MMUCache control behavior", func() {
	var (
		engine      timing.Engine
		comp        *Comp
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	build := func() {
		spec := DefaultSpec()
		spec.NumBlocks = 1
		spec.NumLevels = 5
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
		for _, p := range []messaging.Port{topPort, bottomPort, controlPort} {
			(&noopConn{}).PlugIn(p)
		}
	}

	makeTranslationReq := func(vAddr uint64) vm.TranslationReq {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Requester")
		req.Dst = topPort.AsRemote()
		req.PID = 1
		req.VAddr = vAddr
		req.DeviceID = 1
		req.TrafficClass = "vm.TranslationReq"
		return req
	}

	makeCtrlReq := func(cmd mem.ControlCommand) mem.ControlReq {
		req := mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = controlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	// makeBottomRsp fabricates the low module's response to a forwarded
	// lookup, so the test can let an outstanding walk complete.
	makeBottomRsp := func(fwd vm.TranslationReq) vm.TranslationRsp {
		rsp := vm.TranslationRsp{
			Page: vm.Page{
				PID: fwd.PID, VAddr: fwd.VAddr, PAddr: 0x5000, Valid: true,
			},
		}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = messaging.RemotePort("LowModule")
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = fwd.ID
		rsp.TrafficClass = "vm.TranslationRsp"
		return rsp
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		build()
	})

	It("waits for outstanding bottom walks before acking Drain", func() {
		const n = 3
		for i := range n {
			topPort.Deliver(makeTranslationReq(uint64(0x1000 + i*0x1000)))
		}

		drain := makeCtrlReq(mem.CmdDrain)
		controlPort.Deliver(drain)

		// The cache forwards every lookup down the Bottom port, but with no
		// responses yet the walks are still outstanding, so Drain must NOT ack.
		forwarded := []vm.TranslationReq{}
		for i := 0; i < 256 && len(forwarded) < n; i++ {
			comp.Tick()
			for {
				out := bottomPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if r, ok := out.(vm.TranslationReq); ok {
					forwarded = append(forwarded, r)
				}
			}
			Expect(controlPort.RetrieveOutgoing()).To(BeNil())
		}
		Expect(forwarded).To(HaveLen(n))
		Expect(comp.State.InflightBottomReqs).To(Equal(n))

		// A few more ticks while the walks are still outstanding: still no ack.
		for range 8 {
			comp.Tick()
			Expect(controlPort.RetrieveOutgoing()).To(BeNil())
		}
		Expect(comp.State.CurrentState).To(Equal(mmuCacheStateDrain))

		// Let every outstanding walk complete.
		for _, fr := range forwarded {
			bottomPort.Deliver(makeBottomRsp(fr))
		}

		// Now the cache can quiesce: every walk is answered up the Top port
		// and only then is the async Drain acked.
		upResponses := 0
		var drainRsp mem.ControlRsp
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			comp.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(vm.TranslationRsp); ok {
					upResponses++
				}
			}
			if out := controlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
					drainFound = true
				}
			}
		}

		Expect(drainFound).To(BeTrue())
		Expect(drainRsp.Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		Expect(upResponses).To(Equal(n))
		Expect(comp.State.InflightBottomReqs).To(Equal(0))
		Expect(topPort.PeekIncoming()).To(BeNil())
		Expect(comp.State.CurrentState).To(Equal(mmuCacheStatePause))
	})

	It("freezes incoming traffic while paused", func() {
		comp.State.CurrentState = mmuCacheStatePause
		topPort.Deliver(makeTranslationReq(0x1000))

		for range 5 {
			comp.Tick()
		}

		// The lookup is neither consumed nor forwarded while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes queued work from any control state",
		func(startState string) {
			topPort.Deliver(makeTranslationReq(0x1000))
			comp.State.CurrentState = startState

			reset := makeCtrlReq(mem.CmdReset)
			controlPort.Deliver(reset)

			var rsp mem.ControlRsp
			found := false
			for i := 0; i < 64 && !found; i++ {
				comp.Tick()
				if out := controlPort.RetrieveOutgoing(); out != nil {
					rsp, found = out.(mem.ControlRsp)
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.CurrentState).To(Equal(mmuCacheStateEnable))
			Expect(topPort.PeekIncoming()).To(BeNil())
			Expect(bottomPort.PeekIncoming()).To(BeNil())
		},
		Entry("from Enable", mmuCacheStateEnable),
		Entry("from Pause", mmuCacheStatePause),
		Entry("from Drain", mmuCacheStateDrain),
	)
})
