package idealmemcontroller

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
func TestIdealMemoryControllerControlBehavior(t *testing.T) {
	var (
		engine        timing.Engine
		sim           *testsim.Simulation
		storage       *mem.Storage
		memController *Comp
		topPort       messaging.Port
		ctrlPort      messaging.Port
	)

	build := func() {
		spec := DefaultSpec()
		spec.Width = 4
		spec.Latency = 10

		memController = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("MemCtrl")
		memControllerPorts := makePorts(sim, "MemCtrl", 16)
		memController.BindPort("Top", memControllerPorts.Top)
		memController.BindPort("Control", memControllerPorts.Control)

		topPort = memController.Ports.Top
		ctrlPort = memController.Ports.Control
		for _, p := range []messaging.Port{topPort, ctrlPort} {
			(&noopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	makeRead := func(addr uint64) messaging.Msg {
		req := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: 4},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),

			TrafficBytes: 12,
		}

		return req
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) messaging.Msg {
		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Ctrl"),
			Dst: ctrlPort.AsRemote(),
		}

		return req
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
		for i := range n {
			topPort.Deliver(makeRead(uint64(i * 64)))
		}
		modelingtest.Tick(memController)
		require.Len(t, memController.State.InflightTransactions, n)

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp messaging.Msg
		drainFound := false
		for i := 0; i < 4096 && !drainFound; i++ {
			modelingtest.Tick(memController)
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
		// Every in-flight read finished, and none remain, by the time the
		// async Drain ack is sent.
		require.Equal(t, n, completed)
		require.Empty(t, memController.State.InflightTransactions)
		require.Equal(t, controlStatePaused, memController.State.ControlState)
	})

	t.Run("freezes incoming traffic while paused", func(t *testing.T) {
		setup1(t)

		memController.State.ControlState = controlStatePaused
		topPort.Deliver(makeRead(0))

		for range 5 {
			modelingtest.Tick(memController)
		}

		// The request is neither consumed nor turned into work, and no
		// response is produced, while paused.
		_, present0 := topPort.PeekIncoming()
		require.True(t, present0)
		require.Empty(t, memController.State.InflightTransactions)
		_, present1 := topPort.RetrieveOutgoing()
		require.False(t, present1)
	})

	t.Run("Reset wipes in-flight state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState controlState) {
			t.Helper()

			setup1(t)

			topPort.Deliver(makeRead(0))
			modelingtest.Tick(memController)
			require.NotEmpty(t, memController.State.InflightTransactions)

			memController.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(memController)
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
			require.Empty(t, memController.State.InflightTransactions)
			require.Equal(t, controlStateEnabled, memController.State.ControlState)
		}
		t.Run("from Enabled", func(t *testing.T) { runCase(t, controlStateEnabled) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, controlStatePaused) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// Draining and already quiescent (no in-flight transactions):
		// completePendingDrain acks the Drain. Control commands are serialized
		// with no preemption, so a Reset queued behind the drain is serviced
		// only after the Drain acks.
		memController.State.ControlState = controlStateDraining
		memController.State.CurrentCmdID = 999
		memController.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		memController.State.InflightTransactions = nil

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 16 {
			modelingtest.Tick(memController)
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
		require.Equal(t, controlStateEnabled, memController.State.ControlState)
	})

	t.Run("drops Top traffic queued at Reset", func(t *testing.T) {
		setup1(t)

		// Queue a read on Top while paused, so it sits unconsumed in the Top
		// port's incoming queue.
		memController.State.ControlState = controlStatePaused
		topPort.Deliver(makeRead(0))
		for range 3 {
			modelingtest.Tick(memController)
		}
		_, present2 := topPort.PeekIncoming()
		require.True(t, present2)
		require.Empty(t, memController.State.InflightTransactions)

		// Reset must drop that queued request. Before the fix it survived and
		// (the control middleware runs before the memory middleware) was
		// consumed in the same tick the controller returned to Enabled,
		// producing a stale response.
		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)
		found := false
		for i := 0; i < 64 && !found; i++ {
			modelingtest.Tick(memController)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdReset {
					require.True(t, rsp.Success)
					found = true
				}
			}
		}
		require.True(t, found)

		// The stale read never became work and never produced a response.
		for range 16 {
			modelingtest.Tick(memController)
			_, present3 := topPort.RetrieveOutgoing()
			require.False(t, present3)
		}
		_, present4 := topPort.PeekIncoming()
		require.False(t, present4)
		require.Empty(t, memController.State.InflightTransactions)
	})
}
