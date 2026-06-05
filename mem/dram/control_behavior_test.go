package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// This file holds Layer-2 control-behavior tests: it asserts the actual
// behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
var _ = Describe("DRAM control behavior", func() {
	var (
		engine   timing.Engine
		storage  *mem.Storage
		comp     *Comp
		topPort  messaging.Port
		ctrlPort messaging.Port
	)

	build := func() {
		comp = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{Storage: storage}).
			Build("DRAM")

		topPort = comp.GetPortByName("Top")
		ctrlPort = comp.GetPortByName("Control")
		for _, p := range []messaging.Port{topPort, ctrlPort} {
			(&noopConn{}).PlugIn(p)
		}
	}

	makeRead := func(addr uint64) mem.ReadReq {
		req := mem.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = addr
		req.AccessByteSize = 4
		req.TrafficBytes = 12
		req.TrafficClass = "mem.ReadReq"
		return req
	}

	makeCtrlReq := func(cmd mem.ControlCommand) mem.ControlReq {
		req := mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	// acceptReads delivers n reads and ticks until all n have been pulled
	// into the transaction queue. DRAM accepts one transaction per tick,
	// so a single Tick is not enough.
	acceptReads := func(n int) {
		for i := range n {
			topPort.Deliver(makeRead(uint64(i * 64)))
		}
		for i := 0; i < 64 && len(comp.State.Transactions) < n; i++ {
			comp.Tick()
		}
		Expect(comp.State.Transactions).To(HaveLen(n))
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(1 * mem.MB)
		build()
	})

	It("drains all in-flight reads before acking Drain", func() {
		const n = 3
		acceptReads(n)

		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp mem.ControlRsp
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			comp.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(mem.DataReadyRsp); ok {
					completed++
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
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
		// Every in-flight read finished, and none remain, by the time the
		// async Drain ack is sent.
		Expect(completed).To(Equal(n))
		Expect(comp.State.Transactions).To(BeEmpty())
		Expect(comp.State.ControlState).To(Equal(control.StatePaused))
	})

	It("freezes incoming traffic while paused", func() {
		comp.State.ControlState = control.StatePaused
		topPort.Deliver(makeRead(0))

		for range 5 {
			comp.Tick()
		}

		// The request is neither consumed nor turned into work, and no
		// response is produced, while paused.
		Expect(topPort.PeekIncoming()).ToNot(BeNil())
		Expect(comp.State.Transactions).To(BeEmpty())
		Expect(topPort.RetrieveOutgoing()).To(BeNil())
	})

	It("does not abort an in-flight Drain when a Pause arrives", func() {
		const n = 2
		for i := range n {
			topPort.Deliver(makeRead(uint64(i * 64)))
		}
		comp.Tick()
		Expect(comp.State.Transactions).ToNot(BeEmpty())

		// Begin draining while work is still in flight.
		drain := makeCtrlReq(mem.CmdDrain)
		ctrlPort.Deliver(drain)
		comp.Tick()
		Expect(comp.State.ControlState).To(Equal(control.StateDraining))

		// A Pause arrives mid-drain. Only Reset preempts an async verb, so the
		// Pause must not freeze the controller and strand the drain.
		pause := makeCtrlReq(mem.CmdPause)
		ctrlPort.Deliver(pause)

		drainAcked, pauseAcked := false, false
		for i := 0; i < 4096 && !drainAcked; i++ {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				r, ok := out.(mem.ControlRsp)
				if !ok {
					continue
				}
				switch r.Command {
				case mem.CmdDrain:
					drainAcked = true
					Expect(r.RspTo).To(Equal(drain.ID))
				case mem.CmdPause:
					pauseAcked = true
				}
			}
		}
		Expect(pauseAcked).To(BeTrue())
		Expect(drainAcked).To(BeTrue())
		Expect(comp.State.Transactions).To(BeEmpty())
		Expect(comp.State.ControlState).To(Equal(control.StatePaused))
	})

	It("completes a pending Drain even when a non-Reset verb is queued", func() {
		// Draining and just quiescent, with a Pause queued in the same window.
		// Only Reset preempts a pending drain; a Pause must let it finish, so
		// the Drain ack is still sent (and the Pause is serviced too).
		comp.State.ControlState = control.StateDraining
		comp.State.CurrentCmdID = 777
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.Transactions = nil

		pause := makeCtrlReq(mem.CmdPause)
		ctrlPort.Deliver(pause)

		drainAcked, pauseAcked := false, false
		for range 8 {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				r, ok := out.(mem.ControlRsp)
				if !ok {
					continue
				}
				switch r.Command {
				case mem.CmdDrain:
					drainAcked = true
					Expect(r.RspTo).To(Equal(uint64(777)))
				case mem.CmdPause:
					pauseAcked = true
				}
			}
		}
		Expect(drainAcked).To(BeTrue())
		Expect(pauseAcked).To(BeTrue())
	})

	It("services a queued Reset before completing a pending Drain", func() {
		// The controller is draining and has just become quiescent (no
		// transactions): completePendingDrain would otherwise ack the Drain.
		comp.State.ControlState = control.StateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.Transactions = nil

		// A Reset is queued in the same window. As the highest-priority verb it
		// must be serviced before the Drain completes, so no stale Drain ack is
		// emitted.
		reset := makeCtrlReq(mem.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []mem.ControlRsp
		for range 4 {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if r, ok := out.(mem.ControlRsp); ok {
					rsps = append(rsps, r)
				}
			}
		}

		// Exactly one ack — the Reset — and never a Drain ack.
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].Command).To(Equal(mem.CmdReset))
		Expect(rsps[0].RspTo).To(Equal(reset.ID))
		Expect(comp.State.ControlState).To(Equal(control.StateEnabled))
	})

	It("zeroes accumulated statistics on Reset", func() {
		comp.State.TotalReadCommands = 7
		comp.State.RowBufferHits = 3
		comp.State.TotalCycles = 100
		comp.State.CompletedReads = 5
		comp.State.BytesRead = 640
		comp.State.TotalWriteLatencyCycles = 42

		reset := makeCtrlReq(mem.CmdReset)
		ctrlPort.Deliver(reset)
		found := false
		for i := 0; i < 8 && !found; i++ {
			comp.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if r, ok := out.(mem.ControlRsp); ok &&
					r.Command == mem.CmdReset {
					found = true
				}
			}
		}

		Expect(found).To(BeTrue())
		// Activity counters have no post-reset traffic, so they are exactly 0.
		Expect(comp.State.TotalReadCommands).To(BeZero())
		Expect(comp.State.RowBufferHits).To(BeZero())
		Expect(comp.State.CompletedReads).To(BeZero())
		Expect(comp.State.BytesRead).To(BeZero())
		Expect(comp.State.TotalWriteLatencyCycles).To(BeZero())
		// TotalCycles is cleared by the reset, then counts only post-reset
		// cycles, so it is far below the pre-reset value rather than carrying
		// it forward.
		Expect(comp.State.TotalCycles).To(BeNumerically("<", 100))
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState control.State) {
			acceptReads(1)

			comp.State.ControlState = startState

			reset := makeCtrlReq(mem.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp mem.ControlRsp
			found := false
			for i := 0; i < 64 && !found; i++ {
				comp.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					if r, ok := out.(mem.ControlRsp); ok {
						rsp = r
						found = true
					}
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.Transactions).To(BeEmpty())
			Expect(comp.State.ControlState).
				To(Equal(control.StateEnabled))
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		Entry("from Draining", control.StateDraining),
	)
})
