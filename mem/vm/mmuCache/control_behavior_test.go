package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// This file holds Layer-2 control-behavior tests for the mmuCache: it asserts
// the actual behavior the universal verbs promise (Drain quiescence, Pause
// freeze, Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The mmuCache differs from the idealmemcontroller reference in two ways:
//   - Its control state is a STRING field comp.State.CurrentState with values
//     mmuCacheStateEnable / mmuCacheStatePause / mmuCacheStateDrain (not the
//     memcontrolprotocol.State enum).
//   - It is a stateless forwarder with no MSHR: a Top lookup is forwarded down
//     the Bottom port (an empty cache always misses), and it does not emit a
//     Top response on its own. So "completion" for the Drain test is the
//     request appearing on the Bottom port's outgoing queue, and Drain
//     quiesces when both Top and Bottom incoming queues are empty.
var _ = Describe("MMUCache control behavior", func() {
	var (
		engine      timing.Engine
		sim         timing.Simulation
		comp        *Comp
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	build := func() {
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
			WithResources(Resources{
				LowModulePort: messaging.RemotePort("LowModule"),
				UpModulePort:  messaging.RemotePort("UpModule"),
			}).
			WithPorts(defaultPorts("MMUCache")).
			Build("MMUCache")

		topPort = comp.Ports.Top
		bottomPort = comp.Ports.Bottom
		controlPort = comp.Ports.Control
		for _, p := range []messaging.Port{topPort, bottomPort, controlPort} {
			(&noopConn{}).PlugIn(p)
		}
	}

	makeTranslationReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: topPort.AsRemote(),

			TrafficClass: "vmprotocol.TranslationReq"}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Ctrl"),
			Dst:          controlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

		return req
	}

	// makeBottomRsp fabricates the low module's response to a forwarded
	// lookup, so the test can let an outstanding walk complete.
	makeBottomRsp := func(fwd messaging.Msg) messaging.Msg {
		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{
				PID:   fwd.Payload.(vmprotocol.TranslationReq).PID,
				VAddr: fwd.Payload.(vmprotocol.TranslationReq).VAddr,
				PAddr: 0x5000, Valid: true,
			},
		},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("LowModule"),
			Dst:          bottomPort.AsRemote(),
			RspTo:        fwd.ID,
			TrafficClass: "vmprotocol.TranslationRsp"}

		return rsp
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		build()
	})

	It("waits for outstanding bottom walks before acking Drain", func() {
		const n = 3
		for i := range n {
			topPort.Deliver(makeTranslationReq(uint64(0x1000 + i*0x1000)))
		}

		// Forward every lookup down the Bottom port while still enabled (Drain
		// itself admits no new Top traffic).
		forwarded := []messaging.Msg{}
		for i := 0; i < 256 && len(forwarded) < n; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := bottomPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(vmprotocol.TranslationReq); ok {

					forwarded = append(forwarded, out)
				}
			}
		}
		Expect(forwarded).To(HaveLen(n))
		Expect(comp.State.OutstandingBottomReqs).To(HaveLen(n))

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		controlPort.Deliver(drain)

		// Let the Drain take effect.
		for i := 0; i < 8 && comp.State.CurrentState != mmuCacheStateDrain; i++ {
			modelingtest.Tick(comp)
		}
		Expect(comp.State.CurrentState).To(Equal(mmuCacheStateDrain))

		// A request delivered while draining must not be admitted (no new
		// forward), and with the walks still outstanding Drain must NOT ack.
		topPort.Deliver(makeTranslationReq(0x9000))
		for range 8 {
			modelingtest.Tick(comp)
			_, present0 := controlPort.RetrieveOutgoing()
			Expect(present0).To(BeFalse())
			_, present1 := bottomPort.RetrieveOutgoing()
			Expect(present1).To(BeFalse())
		}
		Expect(comp.State.OutstandingBottomReqs).To(HaveLen(n))
		_, present2 := topPort.PeekIncoming()
		Expect(present2).To(BeTrue()) // late req still queued

		// Let every outstanding walk complete.
		for _, fr := range forwarded {
			bottomPort.Deliver(makeBottomRsp(fr))
		}

		// Now the cache can quiesce: every walk is answered up the Top port
		// and only then is the async Drain acked.
		upResponses := 0
		var drainRsp messaging.Msg
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := topPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(vmprotocol.TranslationRsp); ok {
					upResponses++
				}
			}
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					drainFound = true
				}

			}
		}

		Expect(drainFound).To(BeTrue())
		Expect(drainRsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
		Expect(drainRsp.RspTo).To(Equal(drain.ID))
		Expect(upResponses).To(Equal(n))
		Expect(comp.State.OutstandingBottomReqs).To(BeEmpty())
		// The request delivered mid-drain was never admitted; it remains
		// queued for after Enable.
		_, present3 := topPort.PeekIncoming()
		Expect(present3).To(BeTrue())
		Expect(comp.State.CurrentState).To(Equal(mmuCacheStatePause))
	})

	It("drops a late bottom response that arrives after Reset", func() {
		// Forward a lookup so a bottom request is outstanding.
		topPort.Deliver(makeTranslationReq(0x1000))
		var fwd messaging.Msg
		gotFwd := false
		for i := 0; i < 64 && !gotFwd; i++ {
			modelingtest.Tick(comp)
			if out, ok := bottomPort.RetrieveOutgoing(); ok {
				_, gotFwd = out.Payload.(vmprotocol.TranslationReq)
				fwd = out
			}
		}
		Expect(gotFwd).To(BeTrue())
		Expect(comp.State.OutstandingBottomReqs).To(HaveLen(1))

		// Reset discards the outstanding walk.
		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		controlPort.Deliver(reset)
		resetAcked := false
		for i := 0; i < 64 && !resetAcked; i++ {
			modelingtest.Tick(comp)
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdReset {
					resetAcked = true
				}

			}
		}
		Expect(resetAcked).To(BeTrue())
		Expect(comp.State.OutstandingBottomReqs).To(BeEmpty())

		// The low module's now-orphaned response arrives. It must be dropped:
		// no panic, and no stale translation emitted up the Top port. Before
		// the fix this repopulated the reset table and replied upward.
		bottomPort.Deliver(makeBottomRsp(fwd))
		for range 16 {
			modelingtest.Tick(comp)
			_, present4 := topPort.RetrieveOutgoing()
			Expect(present4).To(BeFalse())
		}
		Expect(comp.State.OutstandingBottomReqs).To(BeEmpty())
	})

	It("freezes incoming traffic while paused", func() {
		comp.State.CurrentState = mmuCacheStatePause
		topPort.Deliver(makeTranslationReq(0x1000))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The lookup is neither consumed nor forwarded while paused.
		_, present5 := topPort.PeekIncoming()
		Expect(present5).To(BeTrue())
		_, present6 := bottomPort.RetrieveOutgoing()
		Expect(present6).To(BeFalse())
	})

	DescribeTable("Reset wipes queued work from any control state",
		func(startState string) {
			topPort.Deliver(makeTranslationReq(0x1000))
			comp.State.CurrentState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			controlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(comp)
				if out, ok := controlPort.RetrieveOutgoing(); ok {
					_, found = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.CurrentState).To(Equal(mmuCacheStateEnable))
			_, present7 := topPort.PeekIncoming()
			Expect(present7).To(BeFalse())
			_, present8 := bottomPort.PeekIncoming()
			Expect(present8).To(BeFalse())
		},
		Entry("from Enable", mmuCacheStateEnable),
		Entry("from Pause", mmuCacheStatePause),
		Entry("from Drain", mmuCacheStateDrain),
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// Draining with the drain ack pending and no in-flight work: the data
		// path flips the state to pause and completePendingDrain acks the
		// Drain. Control commands are serialized with no preemption, so a Reset
		// queued behind the drain is serviced only after the Drain acks.
		comp.State.CurrentState = mmuCacheStateDrain
		comp.State.PendingDrainRsp = true
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		controlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 32 {
			modelingtest.Tick(comp)
			for {
				out, ok := controlPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(memcontrolprotocol.Rsp); ok {

					rsps = append(rsps, out)
				}
			}
		}

		Expect(rsps).To(HaveLen(2))
		Expect(rsps[0].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdDrain))
		Expect(rsps[0].RspTo).To(Equal(uint64(999)))
		Expect(rsps[1].Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
		Expect(rsps[1].RspTo).To(Equal(reset.ID))
		Expect(comp.State.CurrentState).To(Equal(mmuCacheStateEnable))
	})
})
