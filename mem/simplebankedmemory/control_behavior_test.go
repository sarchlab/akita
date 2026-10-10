package simplebankedmemory

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
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
		sim      timing.Simulation
		storage  *mem.Storage
		comp     *Comp
		topPort  messaging.Port
		ctrlPort messaging.Port
	)

	build := func() {
		comp = Definition.Builder().
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("BankedMem")
		compPorts := makePorts("BankedMem", 16, 16)
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Control", compPorts.Control)

		topPort = comp.Ports.Top
		ctrlPort = comp.Ports.Control
		for _, p := range []messaging.Port{topPort, ctrlPort} {
			(&noopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

	}

	makeRead := func(index int) messaging.Msg {
		return makeReadReq(sim, messaging.RemotePort("Agent"), topPort.AsRemote(), index)
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Ctrl"),
			Dst:          ctrlPort.AsRemote(),
			TrafficClass: "memcontrolprotocol.Req"}

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
		sim = modeling.NewStandaloneSimulation(engine)
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
		modelingtest.Tick(comp)
		modelingtest.Tick(comp)
		Expect(allBanksQuiescent()).To(BeFalse())

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp messaging.Msg
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := topPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(memprotocol.DataReadyRsp); ok {
					completed++
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
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
		// Every in-flight read finished, and every bank is quiescent, by the
		// time the async Drain ack is sent. Counting completions at the ack
		// moment proves Drain did not ack early.
		Expect(completed).To(Equal(n))
		Expect(allBanksQuiescent()).To(BeTrue())
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StatePaused))
	})

	It("freezes incoming traffic while paused", func() {
		comp.State.ControlState = memcontrolprotocol.StatePaused
		topPort.Deliver(makeRead(0))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The request is neither consumed nor turned into work, and no
		// response is produced, while paused.
		_, present0 := topPort.PeekIncoming()
		Expect(present0).To(BeTrue())
		Expect(allBanksQuiescent()).To(BeTrue())
		_, present1 := topPort.RetrieveOutgoing()
		Expect(present1).To(BeFalse())
	})

	DescribeTable("Reset wipes in-flight state from any control state",
		func(startState memcontrolprotocol.State) {
			topPort.Deliver(makeRead(0))
			modelingtest.Tick(comp)
			modelingtest.Tick(comp)
			Expect(allBanksQuiescent()).To(BeFalse())

			comp.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(comp)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					if _, ok := out.Payload.(memcontrolprotocol.Rsp); ok {

						rsp = out

						found = true
					}
				}
			}

			Expect(found).To(BeTrue())
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Payload.(memcontrolprotocol.Rsp).Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
			// Reset rebuilds the banks, so all in-flight work is gone.
			Expect(allBanksQuiescent()).To(BeTrue())
			Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))

			// No leftover completion is produced from the wiped-out read.
			completion := false
			for range 8 {
				modelingtest.Tick(comp)
				for {
					out, ok := topPort.RetrieveOutgoing()
					if !ok {
						break
					}
					if _, ok := out.Payload.(memprotocol.DataReadyRsp); ok {
						completion = true
					}
				}
			}
			Expect(completion).To(BeFalse())
		},
		Entry("from Enabled", memcontrolprotocol.StateEnabled),
		Entry("from Paused", memcontrolprotocol.StatePaused),
		// The Draining case is covered separately by the test below, since
		// control commands are serialized: a Reset queued while draining is
		// serviced only after the pending Drain acks (no preemption).
	)

	It("completes a pending Drain before servicing a queued Reset", func() {
		// Draining and already quiescent (banks idle): completePendingDrain
		// acks the Drain. Control commands are serialized with no preemption,
		// so a Reset queued behind the drain is serviced only after the Drain
		// acks.
		comp.State.ControlState = memcontrolprotocol.StateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(comp)
			for {
				out, ok := ctrlPort.RetrieveOutgoing()
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
		Expect(comp.State.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
	})
})
