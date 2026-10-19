package dram

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
func TestDRAMControlBehavior(t *testing.T) {
	var (
		engine   timing.Engine
		sim      *testsim.Simulation
		storage  *mem.Storage
		comp     *Comp
		topPort  messaging.Port
		ctrlPort messaging.Port
	)

	build := func() {
		comp = modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(sim).
			WithResources(Resources{Storage: storage}).
			Build("DRAM")
		compPorts := defaultPorts(sim, "DRAM", 16)
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

	// acceptReads delivers n reads and ticks until all n have been pulled
	// into the transaction queue. DRAM accepts one transaction per tick,
	// so a single Tick is not enough.
	acceptReads := func(t *testing.T, n int) {
		t.Helper()

		for i := range n {
			topPort.Deliver(makeRead(uint64(i * 64)))
		}
		for i := 0; i < 64 && len(comp.State.Transactions) < n; i++ {
			modelingtest.Tick(comp)
		}
		require.Len(t, comp.State.Transactions, n)
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
		acceptReads(t, n)

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
		// Every in-flight read finished, and none remain, by the time the
		// async Drain ack is sent.
		require.Equal(t, n, completed)
		require.Empty(t, comp.State.Transactions)
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
		require.Empty(t, comp.State.Transactions)
		_, present1 := topPort.RetrieveOutgoing()
		require.False(t, present1)
	})

	t.Run("does not abort an in-flight Drain when a Pause arrives", func(t *testing.T) {
		setup1(t)

		const n = 2
		for i := range n {
			topPort.Deliver(makeRead(uint64(i * 64)))
		}
		modelingtest.Tick(comp)
		require.NotEmpty(t, comp.State.Transactions)

		// Begin draining while work is still in flight.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)
		modelingtest.Tick(comp)
		require.Equal(t, controlStateDraining, comp.State.ControlState)

		// A Pause arrives mid-drain. Control commands are serialized, so the
		// Pause stays queued until the drain finishes; it must not freeze the
		// controller and strand the drain.
		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)

		drainAcked, pauseAcked := false, false
		for i := 0; i < 4096 && !drainAcked; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := ctrlPort.RetrieveOutgoing()
				if !ok {
					break
				}
				_, ok = out.Payload.(memcontrolprotocol.Rsp)
				r := out
				if !ok {
					continue
				}
				switch r.Payload.(memcontrolprotocol.Rsp).Command {
				case memcontrolprotocol.CmdDrain:
					drainAcked = true
					require.Equal(t, drain.ID, r.RspTo)
				case memcontrolprotocol.CmdPause:
					pauseAcked = true
				}
			}
		}
		require.True(t, pauseAcked)
		require.True(t, drainAcked)
		require.Empty(t, comp.State.Transactions)
		require.Equal(t, controlStatePaused, comp.State.ControlState)
	})

	t.Run("completes a pending Drain even when a non-Reset verb is queued", func(t *testing.T) {
		setup1(t)

		// Draining and just quiescent, with a Pause queued in the same window.
		// Commands are serialized: the in-flight Drain finishes and acks first,
		// then the queued Pause is serviced.
		comp.State.ControlState = controlStateDraining
		comp.State.CurrentCmdID = 777
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.Transactions = nil

		pause := makeCtrlReq(memcontrolprotocol.CmdPause)
		ctrlPort.Deliver(pause)

		drainAcked, pauseAcked := false, false
		for range 8 {
			modelingtest.Tick(comp)
			for {
				out, ok := ctrlPort.RetrieveOutgoing()
				if !ok {
					break
				}
				_, ok = out.Payload.(memcontrolprotocol.Rsp)
				r := out
				if !ok {
					continue
				}
				switch r.Payload.(memcontrolprotocol.Rsp).Command {
				case memcontrolprotocol.CmdDrain:
					drainAcked = true
					require.Equal(t, uint64(777), r.RspTo)
				case memcontrolprotocol.CmdPause:
					pauseAcked = true
				}
			}
		}
		require.True(t, drainAcked)
		require.True(t, pauseAcked)
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// The controller is draining and has just become quiescent (no
		// transactions): completePendingDrain acks the Drain.
		comp.State.ControlState = controlStateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.Transactions = nil

		// A Reset is queued in the same window. Control commands are serialized
		// with no preemption: the in-flight Drain finishes and acks first, then
		// the queued Reset runs.
		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 4 {
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

		// Two acks, in order: the Drain completion (RspTo 999) then the Reset.
		require.Len(t, rsps, 2)
		require.Equal(t, memcontrolprotocol.CmdDrain, rsps[0].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, uint64(999), rsps[0].RspTo)
		require.Equal(t, memcontrolprotocol.CmdReset, rsps[1].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, reset.ID, rsps[1].RspTo)
		require.Equal(t, controlStateEnabled, comp.State.ControlState)
	})

	t.Run("zeroes accumulated statistics on Reset", func(t *testing.T) {
		setup1(t)

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
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if r, ok := out.Payload.(memcontrolprotocol.Rsp); ok && r.
					Command == memcontrolprotocol.CmdReset {
					found = true
				}
			}
		}

		require.True(t, found)
		// Activity counters have no post-reset traffic, so they are exactly 0.
		require.Zero(t, comp.State.TotalReadCommands)
		require.Zero(t, comp.State.RowBufferHits)
		require.Zero(t, comp.State.CompletedReads)
		require.Zero(t, comp.State.BytesRead)
		require.Zero(t, comp.State.TotalWriteLatencyCycles)
		// TotalCycles is cleared by the reset, then counts only post-reset
		// cycles, so it is far below the pre-reset value rather than carrying
		// it forward.
		require.Less(t, comp.State.TotalCycles, uint64(100))
	})

	t.Run("clears in-flight pending completions on Reset", func(t *testing.T) {
		setup1(t)

		// Seed a pending read/write completion with an absolute completion tick
		// in the future and a high tick count, as if reset arrived mid-flight.
		comp.State.TickCount = 500
		comp.State.PendingCompletions = []pendingCompletion{
			{CompletionTick: 1000, Ref: subTransRef{TxID: 1, SubIndex: 0}},
		}

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		ctrlPort.Deliver(reset)
		found := false
		for i := 0; i < 8 && !found; i++ {
			modelingtest.Tick(comp)
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if r, ok := out.Payload.(memcontrolprotocol.Rsp); ok && r.
					Command == memcontrolprotocol.CmdReset {
					found = true
				}
			}
		}

		require.True(t, found)
		// Without clearing, the stale entry would keep bankTickMW reporting
		// progress (len(PendingCompletions) > 0) until TickCount climbed back to
		// 1000, spinning a freshly-reset controller on pre-reset work.
		require.Empty(t, comp.State.PendingCompletions)
	})

	t.Run("Reset wipes in-flight state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState controlState) {
			t.Helper()

			setup1(t)

			acceptReads(t, 1)

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
			require.Empty(t, comp.State.Transactions)
			require.Equal(t, controlStateEnabled, comp.State.ControlState)
		}
		t.Run("from Enabled", func(t *testing.T) { runCase(t, controlStateEnabled) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, controlStatePaused) })
	})
}
