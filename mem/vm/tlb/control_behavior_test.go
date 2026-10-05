package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests for the TLB: it asserts the
// actual behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The TLB is downstream-dependent: a lookup miss creates an MSHR entry and
// forwards a request out the "Bottom" port; the request does not complete
// until a matching vmprotocol.TranslationRsp is fed back on "Bottom". The Drain test
// below drives real completion through that handshake.
var _ = Describe("TLB control behavior", func() {
	var (
		engine      timing.Engine
		sim         timing.Simulation
		tlbComp     *Comp
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
		remotePort  = messaging.RemotePort("MMU")
	)

	build := func() {
		spec := Definition.DefaultSpec

		tlbComp = Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: remotePort,
				},
			}).
			WithPorts(defaultPorts("TLB")).
			Build("TLB")

		topPort = tlbComp.Ports.Top
		bottomPort = tlbComp.Ports.Bottom
		controlPort = tlbComp.Ports.Control
		for _, p := range []messaging.Port{topPort, bottomPort, controlPort} {
			(&ccNoopConn{}).PlugIn(p)
		}
	}

	makeLookup := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
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

	// makeBottomRsp builds the vmprotocol.TranslationRsp that retires the MSHR entry
	// for the forwarded bottom request. parseBottom matches the response to an
	// MSHR entry by the resolved page's PID/VAddr, so those must equal the
	// request's PID/VAddr; RspTo is set to the forwarded request's ID to mirror
	// the real downstream handshake.
	makeBottomRsp := func(req messaging.Msg) messaging.Msg {
		page := vm.Page{
			PID:   req.Payload.(vmprotocol.TranslationReq).PID,
			VAddr: req.Payload.(vmprotocol.TranslationReq).VAddr,
			PAddr: req.Payload.(vmprotocol.TranslationReq).VAddr + 0x10000,
			Valid: true,
		}
		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{Page: page},
			ID:           sim.NewID(),
			Src:          remotePort,
			Dst:          bottomPort.AsRemote(),
			RspTo:        req.ID,
			TrafficClass: "vmprotocol.TranslationRsp"}

		return rsp
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = modeling.NewStandaloneSimulation(engine)
		build()
	})

	It("drains all in-flight misses before acking Drain", func() {
		const n = 2

		// Deliver N distinct-VAddr lookups that all miss (fresh TLB, distinct
		// pages so each gets its own MSHR entry).
		lookups := []messaging.Msg{
			makeLookup(0x0),
			makeLookup(0x1000),
		}
		for _, req := range lookups {
			topPort.Deliver(req)
		}

		// Tick until both misses are in flight: 2 MSHR entries created and 2
		// requests forwarded out Bottom. Capture the forwarded request IDs.
		var bottomReqs []messaging.Msg
		for i := 0; i < 64 &&
			(len(tlbComp.State.MSHREntries) < n || len(bottomReqs) < n); i++ {
			modelingtest.Tick(tlbComp)
			for {
				out, ok := bottomPort.RetrieveOutgoing()
				if !ok {
					break
				}
				bottomReqs = append(bottomReqs, out)
			}
		}

		// In-flight is genuinely present before Drain, so the test has teeth.
		Expect(tlbComp.State.MSHREntries).To(HaveLen(n))
		Expect(bottomReqs).To(HaveLen(n))
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeFalse())

		// Issue Drain.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		controlPort.Deliver(drain)

		// Negative phase: without feeding responses, Drain must NOT complete.
		var drainRsp messaging.Msg
		drainFound := false
		for range 5 {
			modelingtest.Tick(tlbComp)
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					drainFound = true
				}

			}
		}

		Expect(drainFound).To(BeFalse())
		Expect(tlbComp.State.TLBState).To(Equal(tlbStateDrain))
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeFalse())

		// Positive phase: feed a matching bottom response for each forwarded
		// request, then tick while counting completed responses on Top and
		// watching Control for the Drain ack.
		for _, req := range bottomReqs {
			bottomPort.Deliver(makeBottomRsp(req))
		}

		completed := 0
		for i := 0; i < 256 && !drainFound; i++ {
			modelingtest.Tick(tlbComp)
			for {
				out, ok := topPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(vmprotocol.TranslationRsp); ok {
					completed++
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
		// Every in-flight miss finishes cleanly before the async Drain ack:
		// handleDrain stays draining until HasRespondingMSHR clears, so the
		// translation response that retires the final MSHR entry reaches Top
		// before the component transitions to Paused.
		Expect(completed).To(Equal(n))
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeTrue())
		Expect(tlbComp.State.TLBState).To(Equal(tlbStatePause))
	})

	It("does not admit new Top traffic while draining", func() {
		// Get a miss in flight so Drain has something to wait for.
		topPort.Deliver(makeLookup(0x0))
		var bottomReq messaging.Msg
		got := false
		for i := 0; i < 64 && !got; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := bottomPort.RetrieveOutgoing(); ok {
				_, got = out.Payload.(vmprotocol.TranslationReq)
				bottomReq = out
			}
		}
		Expect(got).To(BeTrue())
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeFalse())

		// Drain, and let it take effect.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		controlPort.Deliver(drain)
		for i := 0; i < 8 && tlbComp.State.TLBState != tlbStateDrain; i++ {
			modelingtest.Tick(tlbComp)
		}
		Expect(tlbComp.State.TLBState).To(Equal(tlbStateDrain))

		// A lookup delivered while draining must not be admitted: no new Bottom
		// forward, it stays queued on Top, and Drain must not ack.
		topPort.Deliver(makeLookup(0x5000))
		for range 8 {
			modelingtest.Tick(tlbComp)
			_, present0 := bottomPort.RetrieveOutgoing()
			Expect(present0).To(BeFalse())
			_, present1 := controlPort.RetrieveOutgoing()
			Expect(present1).To(BeFalse())
		}
		Expect(tlbComp.State.TLBState).To(Equal(tlbStateDrain))
		_, present2 := topPort.PeekIncoming()
		Expect(present2).To(BeTrue()) // late lookup still queued

		// Completing the in-flight miss lets Drain finish; the late lookup is
		// still queued for after Enable.
		bottomPort.Deliver(makeBottomRsp(bottomReq))
		drainFound := false
		for i := 0; i < 256 && !drainFound; i++ {
			modelingtest.Tick(tlbComp)
			for {
				if _, ok := topPort.RetrieveOutgoing(); !ok {
					break
				}
			}
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainFound = true
				}

			}
		}
		Expect(drainFound).To(BeTrue())
		Expect(tlbComp.State.TLBState).To(Equal(tlbStatePause))
		_, present3 := topPort.PeekIncoming()
		Expect(present3).To(BeTrue()) // late lookup survived
	})

	It("drops a stale bottom translation that arrives after Reset", func() {
		// First lookup misses and forwards a bottom request (MSHR entry A).
		topPort.Deliver(makeLookup(0x100))
		var reqA messaging.Msg
		gotA := false
		for i := 0; i < 64 && !gotA; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := bottomPort.RetrieveOutgoing(); ok {
				_, gotA = out.Payload.(vmprotocol.TranslationReq)
				reqA = out
			}
		}
		Expect(gotA).To(BeTrue())

		// Reset discards the outstanding walk and the TLB contents.
		rst := makeCtrlReq(memcontrolprotocol.CmdReset)
		controlPort.Deliver(rst)
		acked := false
		for i := 0; i < 64 && !acked; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if r, ok := out.Payload.(memcontrolprotocol.Rsp); ok && r.
					Command == memcontrolprotocol.CmdReset {
					acked = true
				}

			}
		}
		Expect(acked).To(BeTrue())

		// A new lookup for the SAME address misses and forwards a fresh bottom
		// request (MSHR entry B) with a different ID.
		topPort.Deliver(makeLookup(0x100))
		var reqB messaging.Msg
		gotB := false
		for i := 0; i < 64 && !gotB; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := bottomPort.RetrieveOutgoing(); ok {
				_, gotB = out.Payload.(vmprotocol.TranslationReq)
				reqB = out
			}
		}
		Expect(gotB).To(BeTrue())
		Expect(reqB.ID).ToNot(Equal(reqA.ID))

		// The stale pre-reset response (for request A) arrives. It must be
		// dropped: no Top response, and the new request's MSHR entry survives.
		bottomPort.Deliver(makeBottomRsp(reqA))
		for range 8 {
			modelingtest.Tick(tlbComp)
			_, present4 := topPort.RetrieveOutgoing()
			Expect(present4).To(BeFalse())
		}
		Expect(mshrIsEntryPresent(
			tlbComp.State.MSHREntries,
			reqB.Payload.(vmprotocol.TranslationReq).PID,
			reqB.Payload.(vmprotocol.TranslationReq).VAddr)).To(BeTrue())

		// The legitimate response (for request B) is accepted and answered.
		bottomPort.Deliver(makeBottomRsp(reqB))
		answered := false
		for i := 0; i < 64 && !answered; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(vmprotocol.TranslationRsp); ok {
					answered = true
				}
			}
		}
		Expect(answered).To(BeTrue())
	})

	It("freezes incoming traffic while paused", func() {
		tlbComp.State.TLBState = tlbStatePause
		topPort.Deliver(makeLookup(0x0))

		for range 5 {
			modelingtest.Tick(tlbComp)
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out Bottom, while paused.
		_, present5 := topPort.PeekIncoming()
		Expect(present5).To(BeTrue())
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeTrue())
		_, present6 := bottomPort.RetrieveOutgoing()
		Expect(present6).To(BeFalse())
	})

	DescribeTable("Reset wipes in-flight MSHR state from any control state",
		func(startState string) {
			// Get one miss in flight: an MSHR entry and a forwarded bottom req.
			topPort.Deliver(makeLookup(0x0))
			for i := 0; i < 64 && mshrIsEmpty(tlbComp.State.MSHREntries); i++ {
				modelingtest.Tick(tlbComp)
			}
			Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeFalse())

			tlbComp.State.TLBState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			controlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(tlbComp)
				if out, ok := controlPort.RetrieveOutgoing(); ok {
					_, found = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			// Reset is a hard reset: the in-flight MSHR entry is discarded
			// (handleReset clears MSHREntries) and the TLB returns to Enabled.
			Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeTrue())
			Expect(tlbComp.State.TLBState).To(Equal(tlbStateEnable))
		},
		Entry("from Enable", tlbStateEnable),
		Entry("from Pause", tlbStatePause),
		// The Drain case is covered separately: while draining, control
		// commands are serialized, so a Reset waits for the Drain to ack first
		// (see "completes a pending Drain before servicing a queued Reset").
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// Draining with the drain ack pending and no in-flight work: the data
		// path flips the state to pause and completePendingDrain acks the
		// Drain. Control commands are serialized with no preemption, so a Reset
		// queued behind the drain is serviced only after the Drain acks.
		tlbComp.State.TLBState = tlbStateDrain
		tlbComp.State.PendingDrainRsp = true
		tlbComp.State.CurrentCmdID = 999
		tlbComp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		controlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 32 {
			modelingtest.Tick(tlbComp)
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
		Expect(tlbComp.State.TLBState).To(Equal(tlbStateEnable))
	})
})
