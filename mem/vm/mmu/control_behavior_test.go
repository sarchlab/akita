package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// This file holds Layer-2 control-behavior tests for the MMU: it asserts the
// actual behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The MMU resolves a translation against its in-process page table without a
// downstream round-trip, so a walk for a MAPPED page is self-completing: it
// produces a addrtransprotocol.Rsp on the Top port and removes itself from
// WalkingTranslations. The tests below rely on that by inserting mapped pages,
// keeping the walk entirely local.
func TestMMUControlBehavior(t *testing.T) {
	var (
		engine    timing.Engine
		sim       *testsim.Simulation
		pageTable *vm.PageTable
		comp      *Comp
		topPort   messaging.Port
		ctrlPort  messaging.Port
	)

	build := func() {
		comp = modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			Build("MMU")
		compPorts := makePorts(sim, "MMU", 16)
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

	// insertMappedPage adds a page that resolves locally: it is not migrating
	// and its DeviceID matches the translation request's DeviceID (0), so the
	// walk never enters the migration path.
	insertMappedPage := func(vAddr uint64) vm.Page {
		page := vm.Page{
			PID:      1,
			VAddr:    vAddr,
			PAddr:    vAddr,
			PageSize: 4096,
			Valid:    true,
			DeviceID: 0,
		}
		pageTable.Insert(page)
		return page
	}

	makeTranslationReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 0},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Agent"),
			Dst: topPort.AsRemote(),
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
		pageTable = vm.NewPageTable(12)
		build()
	}

	t.Run("drains all in-flight translations before acking Drain", func(t *testing.T) {
		setup1(t)

		const n = 3
		for i := range n {
			vAddr := uint64(0x1000 * (i + 1))
			insertMappedPage(vAddr)
			topPort.Deliver(makeTranslationReq(vAddr))
		}

		// Tick until every request has been parsed into a walk.
		for i := 0; i < 4096 &&
			len(comp.State.WalkingTranslations) != n; i++ {
			modelingtest.Tick(comp)
		}
		require.Len(t, comp.State.WalkingTranslations, n)

		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		ctrlPort.Deliver(drain)

		completed := 0
		var drainRsp messaging.Msg
		gotDrainRsp := false
		for i := 0; i < 4096 && !gotDrainRsp; i++ {
			modelingtest.Tick(comp)
			for {
				out, ok := topPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(addrtransprotocol.Rsp); ok {
					completed++
				}
			}
			if out, ok := ctrlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					gotDrainRsp = true
				}
			}
		}

		require.True(t, gotDrainRsp)
		require.True(t, drainRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, drain.ID, drainRsp.RspTo)
		// Every in-flight walk finished, and none remain, by the time the
		// async Drain ack is sent.
		require.Equal(t, n, completed)
		require.Empty(t, comp.State.WalkingTranslations)
		require.Equal(t, controlStatePaused, comp.State.ControlState)
	})

	t.Run("freezes incoming translations while paused", func(t *testing.T) {
		setup1(t)

		comp.State.ControlState = controlStatePaused

		insertMappedPage(0x1000)
		topPort.Deliver(makeTranslationReq(0x1000))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The request is neither consumed nor turned into a walk, and no
		// response is produced, while paused.
		_, present0 := topPort.PeekIncoming()
		require.True(t, present0)
		require.Empty(t, comp.State.WalkingTranslations)
		_, present1 := topPort.RetrieveOutgoing()
		require.False(t, present1)
	})

	t.Run("Reset wipes in-flight walk state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState controlState) {
			t.Helper()

			setup1(t)

			insertMappedPage(0x1000)
			topPort.Deliver(makeTranslationReq(0x1000))

			// Tick until the request is parsed into a walk in flight.
			for i := 0; i < 4096 &&
				len(comp.State.WalkingTranslations) == 0; i++ {
				modelingtest.Tick(comp)
			}
			require.NotEmpty(t, comp.State.WalkingTranslations)

			comp.State.ControlState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			var rsp messaging.Msg
			gotRsp := false
			for i := 0; i < 64 && !gotRsp; i++ {
				modelingtest.Tick(comp)
				if out, ok := ctrlPort.RetrieveOutgoing(); ok {
					_, gotRsp = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			require.True(t, gotRsp)
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, reset.ID, rsp.RspTo)
			require.Empty(t, comp.State.WalkingTranslations)
			require.Equal(t, controlStateEnabled, comp.State.ControlState)
		}
		t.Run("from Enabled", func(t *testing.T) { runCase(t, controlStateEnabled) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, controlStatePaused) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// The component is draining and already quiescent (no in-flight
		// walks): completePendingDrain acks the Drain. Control commands are
		// serialized with no preemption, so a Reset queued behind the drain is
		// serviced only after the Drain acks.
		comp.State.ControlState = controlStateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.WalkingTranslations = nil

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

		// Two acks, in order: the Drain completion (RspTo 999) then the Reset.
		require.Len(t, rsps, 2)
		require.Equal(t, memcontrolprotocol.CmdDrain, rsps[0].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, uint64(999), rsps[0].RspTo)
		require.Equal(t, memcontrolprotocol.CmdReset, rsps[1].Payload.(memcontrolprotocol.Rsp).Command)
		require.Equal(t, reset.ID, rsps[1].RspTo)
		require.Equal(t, controlStateEnabled, comp.State.ControlState)
	})
}
