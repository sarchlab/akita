package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
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
		reg := modeling.NewStandaloneRegistrar(engine)
		comp = MakeBuilder().
			WithRegistrar(reg).
			WithResources(Resources{Storage: storage}).
			Build("DRAM")

		for _, name := range []string{"Top", "Control"} {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(comp).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			comp.AssignPort(name, p)
		}

		topPort = comp.GetPortByName("Top")
		ctrlPort = comp.GetPortByName("Control")
		for _, p := range []messaging.Port{topPort, ctrlPort} {
			(&noopConn{}).PlugIn(p)
		}
	}

	makeRead := func(addr uint64) memprotocol.ReadReq {
		req := memprotocol.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Agent")
		req.Dst = topPort.AsRemote()
		req.Address = addr
		req.AccessByteSize = 4
		req.TrafficBytes = 12
		req.TrafficClass = "memprotocol.ReadReq"
		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) memcontrolprotocol.Req {
		req := memcontrolprotocol.Req{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
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

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp memcontrolprotocol.Rsp
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			comp.Tick()
			for {
				out := topPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if _, ok := out.(memprotocol.DataReadyRsp); ok {
					completed++
				}
			}
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if rsp, ok := out.(memcontrolprotocol.Rsp); ok &&
					rsp.Command == memcontrolprotocol.CmdDrain {
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
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
	})

	It("freezes incoming traffic while paused", func() {
		comp.State.ControlState = memcontrolprotocol.StatePaused
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
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)
		comp.Tick()
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StateDraining))

		// A Pause arrives mid-drain. Control commands are serialized, so the
		// Pause stays queued until the drain finishes; it must not freeze the
		// controller and strand the drain.
		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)

		drainAcked, pauseAcked := false, false
		for i := 0; i < 4096 && !drainAcked; i++ {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				r, ok := out.(memcontrolprotocol.Rsp)
				if !ok {
					continue
				}
				switch r.Command {
				case memcontrolprotocol.CmdDrain:
					drainAcked = true
					Expect(r.RspTo).To(Equal(drain.ID))
				case memcontrolprotocol.CmdPause:
					pauseAcked = true
				}
			}
		}
		Expect(pauseAcked).To(BeTrue())
		Expect(drainAcked).To(BeTrue())
		Expect(comp.State.Transactions).To(BeEmpty())
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
	})

	It("completes a pending Drain even when a non-Reset verb is queued", func() {
		// Draining and just quiescent, with a Pause queued in the same window.
		// Commands are serialized: the in-flight Drain finishes and acks first,
		// then the queued Pause is serviced.
		comp.State.ControlState = memcontrolprotocol.StateDraining
		comp.State.CurrentCmdID = 777
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.Transactions = nil

		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)

		drainAcked, pauseAcked := false, false
		for range 8 {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				r, ok := out.(memcontrolprotocol.Rsp)
				if !ok {
					continue
				}
				switch r.Command {
				case memcontrolprotocol.CmdDrain:
					drainAcked = true
					Expect(r.RspTo).To(Equal(uint64(777)))
				case memcontrolprotocol.CmdPause:
					pauseAcked = true
				}
			}
		}
		Expect(drainAcked).To(BeTrue())
		Expect(pauseAcked).To(BeTrue())
	})

	It("completes a pending Drain before servicing a queued Reset", func() {
		// The controller is draining and has just become quiescent (no
		// transactions): completePendingDrain acks the Drain.
		comp.State.ControlState = memcontrolprotocol.StateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.Transactions = nil

		// A Reset is queued in the same window. Control commands are serialized
		// with no preemption: the in-flight Drain finishes and acks first, then
		// the queued Reset runs.
		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []memcontrolprotocol.Rsp
		for range 4 {
			comp.Tick()
			for {
				out := ctrlPort.RetrieveOutgoing()
				if out == nil {
					break
				}
				if r, ok := out.(memcontrolprotocol.Rsp); ok {
					rsps = append(rsps, r)
				}
			}
		}

		// Two acks, in order: the Drain completion (RspTo 999) then the Reset.
		Expect(rsps).To(HaveLen(2))
		Expect(rsps[0].Command).To(Equal(memcontrolprotocol.CmdDrain))
		Expect(rsps[0].RspTo).To(Equal(uint64(999)))
		Expect(rsps[1].Command).To(Equal(memcontrolprotocol.CmdReset))
		Expect(rsps[1].RspTo).To(Equal(reset.ID))
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
	})

	It("zeroes accumulated statistics on Reset", func() {
		comp.State.TotalReadCommands = 7
		comp.State.RowBufferHits = 3
		comp.State.TotalCycles = 100
		comp.State.CompletedReads = 5
		comp.State.BytesRead = 640
		comp.State.TotalWriteLatencyCycles = 42

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)
		found := false
		for i := 0; i < 8 && !found; i++ {
			comp.Tick()
			if out := ctrlPort.RetrieveOutgoing(); out != nil {
				if r, ok := out.(memcontrolprotocol.Rsp); ok &&
					r.Command == memcontrolprotocol.CmdReset {
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
		func(startState memcontrolprotocol.State) {
			acceptReads(1)

			comp.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp memcontrolprotocol.Rsp
			found := false
			for i := 0; i < 64 && !found; i++ {
				comp.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					if r, ok := out.(memcontrolprotocol.Rsp); ok {
						rsp = r
						found = true
					}
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			Expect(comp.State.Transactions).To(BeEmpty())
			Expect(comp.State.ControlState).
				To(Equal(memcontrolprotocol.StateEnabled))
		},
		// Reset from a draining state is covered separately ("completes a
		// pending Drain before servicing a queued Reset"): under serialization
		// it does not run immediately but waits for the drain to ack.
		Entry("from Enabled", memcontrolprotocol.StateEnabled),
		Entry("from Paused", memcontrolprotocol.StatePaused),
	)
})
