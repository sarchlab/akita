package tlb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests for the TLB: it asserts the
// actual behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The TLB is downstream-dependent: a lookup miss creates an MSHR entry and
// forwards a request out the "Bottom" port; the request does not complete
// until a matching vm.TranslationRsp is fed back on "Bottom". The Drain test
// below drives real completion through that handshake.
var _ = Describe("TLB control behavior", func() {
	var (
		engine      timing.Engine
		tlbComp     *Comp
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
		remotePort  = messaging.RemotePort("MMU")
	)

	build := func() {
		spec := DefaultSpec()

		tlbComp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{
				TranslationProviderMapper: &mem.SinglePortMapper{
					Port: remotePort,
				},
			}).
			Build("TLB")

		topPort = tlbComp.GetPortByName("Top")
		bottomPort = tlbComp.GetPortByName("Bottom")
		controlPort = tlbComp.GetPortByName("Control")
		for _, p := range []messaging.Port{topPort, bottomPort, controlPort} {
			(&ccNoopConn{}).PlugIn(p)
		}
	}

	makeLookup := func(vAddr uint64) vm.TranslationReq {
		req := vm.TranslationReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
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

	// makeBottomRsp builds the vm.TranslationRsp that retires the MSHR entry
	// for the forwarded bottom request. parseBottom matches the response to an
	// MSHR entry by the resolved page's PID/VAddr, so those must equal the
	// request's PID/VAddr; RspTo is set to the forwarded request's ID to mirror
	// the real downstream handshake.
	makeBottomRsp := func(req vm.TranslationReq) vm.TranslationRsp {
		page := vm.Page{
			PID:   req.PID,
			VAddr: req.VAddr,
			PAddr: req.VAddr + 0x10000,
			Valid: true,
		}
		rsp := vm.TranslationRsp{Page: page}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = remotePort
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = req.ID
		rsp.TrafficClass = "vm.TranslationRsp"
		return rsp
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		build()
	})

	It("drains all in-flight misses before acking Drain", func() {
		const n = 2

		// Deliver N distinct-VAddr lookups that all miss (fresh TLB, distinct
		// pages so each gets its own MSHR entry).
		lookups := []vm.TranslationReq{
			makeLookup(0x0),
			makeLookup(0x1000),
		}
		for _, req := range lookups {
			topPort.Deliver(req)
		}

		// Tick until both misses are in flight: 2 MSHR entries created and 2
		// requests forwarded out Bottom. Capture the forwarded request IDs.
		var bottomReqs []vm.TranslationReq
		for i := 0; i < 64 &&
			(len(tlbComp.State.MSHREntries) < n || len(bottomReqs) < n); i++ {
			tlbComp.Tick()
			for {
				out := bottomPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				bottomReqs = append(bottomReqs, out.(vm.TranslationReq))
			}
		}

		// In-flight is genuinely present before Drain, so the test has teeth.
		Expect(tlbComp.State.MSHREntries).To(HaveLen(n))
		Expect(bottomReqs).To(HaveLen(n))
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeFalse())

		// Issue Drain.
		drain := makeCtrlReq(mem.CmdDrain)
		controlPort.Deliver(drain)

		// Negative phase: without feeding responses, Drain must NOT complete.
		var drainRsp mem.ControlRsp
		drainFound := false
		for range 5 {
			tlbComp.Tick()
			if out := controlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainRsp = rsp
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
			tlbComp.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(vm.TranslationRsp); ok {
					completed++
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
		var bottomReq vm.TranslationReq
		got := false
		for i := 0; i < 64 && !got; i++ {
			tlbComp.Tick()
			if out := bottomPort.RetrieveOutgoing(); out != nil {
				bottomReq, got = out.(vm.TranslationReq)
			}
		}
		Expect(got).To(BeTrue())
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeFalse())

		// Drain, and let it take effect.
		drain := makeCtrlReq(mem.CmdDrain)
		controlPort.Deliver(drain)
		for i := 0; i < 8 && tlbComp.State.TLBState != tlbStateDrain; i++ {
			tlbComp.Tick()
		}
		Expect(tlbComp.State.TLBState).To(Equal(tlbStateDrain))

		// A lookup delivered while draining must not be admitted: no new Bottom
		// forward, it stays queued on Top, and Drain must not ack.
		topPort.Deliver(makeLookup(0x5000))
		for range 8 {
			tlbComp.Tick()
			Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
			Expect(controlPort.RetrieveOutgoing()).To(BeNil())
		}
		Expect(tlbComp.State.TLBState).To(Equal(tlbStateDrain))
		Expect(topPort.PeekIncoming()).ToNot(BeNil()) // late lookup still queued

		// Completing the in-flight miss lets Drain finish; the late lookup is
		// still queued for after Enable.
		bottomPort.Deliver(makeBottomRsp(bottomReq))
		drainFound := false
		for i := 0; i < 256 && !drainFound; i++ {
			tlbComp.Tick()
			for {
				if topPort.RetrieveOutgoing() == nil {
					break
				}
			}
			if out := controlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(mem.ControlRsp); ok &&
					rsp.Command == mem.CmdDrain {
					drainFound = true
				}
			}
		}
		Expect(drainFound).To(BeTrue())
		Expect(tlbComp.State.TLBState).To(Equal(tlbStatePause))
		Expect(topPort.PeekIncoming()).ToNot(BeNil()) // late lookup survived
	})

	It("drops a stale bottom translation that arrives after Reset", func() {
		// First lookup misses and forwards a bottom request (MSHR entry A).
		topPort.Deliver(makeLookup(0x100))
		var reqA vm.TranslationReq
		gotA := false
		for i := 0; i < 64 && !gotA; i++ {
			tlbComp.Tick()
			if out := bottomPort.RetrieveOutgoing(); out != nil {
				reqA, gotA = out.(vm.TranslationReq)
			}
		}
		Expect(gotA).To(BeTrue())

		// Reset discards the outstanding walk and the TLB contents.
		rst := makeCtrlReq(mem.CmdReset)
		controlPort.Deliver(rst)
		acked := false
		for i := 0; i < 64 && !acked; i++ {
			tlbComp.Tick()
			if out := controlPort.RetrieveOutgoing(); out != nil {
				if r, ok := out.(mem.ControlRsp); ok &&
					r.Command == mem.CmdReset {
					acked = true
				}
			}
		}
		Expect(acked).To(BeTrue())

		// A new lookup for the SAME address misses and forwards a fresh bottom
		// request (MSHR entry B) with a different ID.
		topPort.Deliver(makeLookup(0x100))
		var reqB vm.TranslationReq
		gotB := false
		for i := 0; i < 64 && !gotB; i++ {
			tlbComp.Tick()
			if out := bottomPort.RetrieveOutgoing(); out != nil {
				reqB, gotB = out.(vm.TranslationReq)
			}
		}
		Expect(gotB).To(BeTrue())
		Expect(reqB.ID).ToNot(Equal(reqA.ID))

		// The stale pre-reset response (for request A) arrives. It must be
		// dropped: no Top response, and the new request's MSHR entry survives.
		bottomPort.Deliver(makeBottomRsp(reqA))
		for range 8 {
			tlbComp.Tick()
			Expect(topPort.RetrieveOutgoing()).To(BeNil())
		}
		Expect(mshrIsEntryPresent(
			tlbComp.State.MSHREntries, reqB.PID, reqB.VAddr)).To(BeTrue())

		// The legitimate response (for request B) is accepted and answered.
		bottomPort.Deliver(makeBottomRsp(reqB))
		answered := false
		for i := 0; i < 64 && !answered; i++ {
			tlbComp.Tick()
			if out := topPort.RetrieveOutgoing(); out != nil {
				if _, ok := out.(vm.TranslationRsp); ok {
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
			tlbComp.Tick()
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out Bottom, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeTrue())
		Expect(bottomPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight MSHR state from any control state",
		func(startState string) {
			// Get one miss in flight: an MSHR entry and a forwarded bottom req.
			topPort.Deliver(makeLookup(0x0))
			for i := 0; i < 64 && mshrIsEmpty(tlbComp.State.MSHREntries); i++ {
				tlbComp.Tick()
			}
			Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeFalse())

			tlbComp.State.TLBState = startState

			reset := makeCtrlReq(mem.CmdReset)
			controlPort.Deliver(reset)

			var rsp mem.ControlRsp
			found := false
			for i := 0; i < 64 && !found; i++ {
				tlbComp.Tick()
				if out := controlPort.RetrieveOutgoing(); out != nil {
					rsp, found = out.(mem.ControlRsp)
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			// Reset is a hard reset: the in-flight MSHR entry is discarded
			// (handleReset clears MSHREntries) and the TLB returns to Enabled.
			Expect(mshrIsEmpty(tlbComp.State.MSHREntries)).To(BeTrue())
			Expect(tlbComp.State.TLBState).To(Equal(tlbStateEnable))
		},
		Entry("from Enable", tlbStateEnable),
		Entry("from Pause", tlbStatePause),
		Entry("from Drain", tlbStateDrain),
	)
})
