package simplebankedmemory

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
//
// Unlike the ideal memory controller, this component has no single
// in-flight slice; work lives spread across per-bank pipelines and
// post-pipeline buffers. Quiescence is therefore checked per bank via
// bankIsQuiescent.
var _ = Describe("Simple Banked Memory control behavior", func() {
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
			Build("BankedMem")

		topPort = comp.GetPortByName("Top")
		ctrlPort = comp.GetPortByName("Control")
		for _, p := range []messaging.Port{topPort, ctrlPort} {
			(&noopConn{}).PlugIn(p)
		}
	}

	makeRead := func(index int) mem.ReadReq {
		return makeReadReq(messaging.RemotePort("Agent"), topPort.AsRemote(), index)
	}

	makeCtrlReq := func(cmd mem.ControlCommand) mem.ControlReq {
		req := mem.ControlReq{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = messaging.RemotePort("Ctrl")
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "mem.ControlReq"
		return req
	}

	allBanksQuiescent := func() bool {
		for i := range comp.State.Banks {
			if !bankIsQuiescent(&comp.State.Banks[i]) {
				return false
			}
		}
		return true
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(1 * mem.MB)
		build()
	})

	It("drains all in-flight reads before acking Drain", func() {
		const n = 3
		// Indices 0, 1, 2 land in distinct banks (bank = index % NumBanks).
		for i := range n {
			topPort.Deliver(makeRead(i))
		}

		// Tick a couple of times so dispatchMW routes the reads into the
		// bank pipelines, putting real work in flight.
		comp.Tick()
		comp.Tick()
		Expect(allBanksQuiescent()).To(BeFalse())

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
		// Every in-flight read finished, and every bank is quiescent, by the
		// time the async Drain ack is sent. Counting completions at the ack
		// moment proves Drain did not ack early.
		Expect(completed).To(Equal(n))
		Expect(allBanksQuiescent()).To(BeTrue())
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
		Expect(allBanksQuiescent()).To(BeTrue())
		Expect(topPort.RetrieveOutgoing()).To(BeNil())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState control.State) {
			topPort.Deliver(makeRead(0))
			comp.Tick()
			comp.Tick()
			Expect(allBanksQuiescent()).To(BeFalse())

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
			// Reset rebuilds the banks, so all in-flight work is gone.
			Expect(allBanksQuiescent()).To(BeTrue())
			Expect(comp.State.ControlState).To(Equal(control.StateEnabled))

			// No leftover completion is produced from the wiped-out read.
			completion := false
			for range 8 {
				comp.Tick()
				for {
					out := topPort.RetrieveOutgoing()
					if out == nil {
						break
					}
					if _, ok := out.(mem.DataReadyRsp); ok {
						completion = true
					}
				}
			}
			Expect(completion).To(BeFalse())
		},
		Entry("from Enabled", control.StateEnabled),
		Entry("from Paused", control.StatePaused),
		// The Draining case is covered separately by the test below, since
		// control commands are serialized: a Reset queued while draining is
		// serviced only after the pending Drain acks (no preemption).
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// Draining and already quiescent (banks idle): completePendingDrain
		// acks the Drain. Control commands are serialized with no preemption,
		// so a Reset queued behind the drain is serviced only after the Drain
		// acks.
		comp.State.ControlState = control.StateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(mem.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []mem.ControlRsp
		for range 16 {
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

		Expect(rsps).To(HaveLen(2))
		Expect(rsps[0].Command).To(Equal(mem.CmdDrain))
		Expect(rsps[0].RspTo).To(Equal(uint64(999)))
		Expect(rsps[1].Command).To(Equal(mem.CmdReset))
		Expect(rsps[1].RspTo).To(Equal(reset.ID))
		Expect(comp.State.ControlState).To(Equal(control.StateEnabled))
	})
})
