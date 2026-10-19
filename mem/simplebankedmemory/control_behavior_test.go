package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
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
func TestSimpleBankedMemoryControlBehavior(t *testing.T) {
	var (
		engine   timing.Engine
		sim      timing.Simulation
		storage  *mem.Storage
		comp     *Comp
		topPort  messaging.Port
		ctrlPort messaging.Port
	)

	build := func() {
		comp = modeling.MakeBuilder[*Comp](DefaultSpec()).
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
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Ctrl"),
			Dst: ctrlPort.AsRemote(),
		}

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

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		storage = mem.NewStorage(1 * mem.MB)
		build()
	}

	t.Run("drains all in-flight reads before acking Drain", func(t *testing.T) {
		setup1(t)

		const n = 3
		// Indices 0, 1, 2 land in distinct banks (bank = index % NumBanks).
		for i := range n {
			topPort.Deliver(makeRead(i))
		}

		// Tick a couple of times so dispatchMW routes the reads into the
		// bank pipelines, putting real work in flight.
		modelingtest.Tick(comp)
		modelingtest.Tick(comp)
		require.False(t, allBanksQuiescent())

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

		require.True(t, drainFound)
		require.True(t, drainRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, drain.ID, drainRsp.RspTo)
		// Every in-flight read finished, and every bank is quiescent, by the
		// time the async Drain ack is sent. Counting completions at the ack
		// moment proves Drain did not ack early.
		require.Equal(t, n, completed)
		require.True(t, allBanksQuiescent())
		require.Equal(t, controlStatePaused, comp.State.ControlState)
	})

	t.Run("freezes incoming traffic while paused", func(t *testing.T) {
		setup1(t)

		comp.State.ControlState = controlStatePaused
		topPort.Deliver(makeRead(0))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The request is neither consumed nor turned into work, and no
		// response is produced, while paused.
		_, present0 := topPort.PeekIncoming()
		require.True(t, present0)
		require.True(t, allBanksQuiescent())
		_, present1 := topPort.RetrieveOutgoing()
		require.False(t, present1)
	})

	t.Run("Reset wipes in-flight state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState controlState) {
			t.Helper()

			setup1(t)

			topPort.Deliver(makeRead(0))
			modelingtest.Tick(comp)
			modelingtest.Tick(comp)
			require.False(t, allBanksQuiescent())

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

			require.True(t, found)
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, reset.ID, rsp.RspTo)
			// Reset rebuilds the banks, so all in-flight work is gone.
			require.True(t, allBanksQuiescent())
			require.Equal(t, controlStateEnabled, comp.State.ControlState)

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
			require.False(t, completion)
		}
		t.Run("from Enabled", func(t *testing.T) { runCase(t, controlStateEnabled) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, controlStatePaused) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// Draining and already quiescent (banks idle): completePendingDrain
		// acks the Drain. Control commands are serialized with no preemption,
		// so a Reset queued behind the drain is serviced only after the Drain
		// acks.
		comp.State.ControlState = controlStateDraining
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

		require.Len(t, rsps, 2)
		require.Equal(t, memcontrolprotocol.CmdDrain, rsps[0].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, uint64(999), rsps[0].RspTo)
		require.Equal(t, memcontrolprotocol.CmdReset, rsps[1].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, reset.ID, rsps[1].RspTo)
		require.Equal(t, controlStateEnabled, comp.State.ControlState)
	})
}
