package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// This file holds Layer-2 control-behavior tests for the GMMU: it asserts the
// actual behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// All translation requests target locally-MAPPED pages (page.DeviceID equal to
// the GMMU's spec.DeviceID), so each walk self-completes via finalizePageWalk
// and emits a *vmprotocol.TranslationRsp on Top without ever sending a remote memory
// request out Bottom. That keeps RemoteMemReqs empty and lets Drain reach
// quiescence purely on the local walk path.
func TestGMMUControlBehavior(t *testing.T) {
	var (
		engine    timing.Engine
		sim       timing.Simulation
		pageTable *vm.PageTable
		comp      *Comp
		topPort   messaging.Port
		ctrlPort  messaging.Port
	)

	const (
		agentPort = messaging.RemotePort("Agent")
		lowModule = messaging.RemotePort("LowModule")
		deviceID  = uint64(0)
	)

	build := func() {
		spec := DefaultSpec()
		spec.DeviceID = deviceID
		// A latency larger than the in-flight count lets every delivered
		// request be parsed into a walk before the first one finalizes, so
		// all N walks can be observed in flight at once.
		spec.Latency = 10
		spec.Bottom = lowModule

		comp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{PageTable: pageTable}).
			Build("GMMU")
		compPorts := defaultPorts("GMMU")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		topPort = comp.Ports.Top
		ctrlPort = comp.Ports.Control
		for _, p := range allPorts(comp) {
			(&noopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	// insertLocalPage maps vAddr to a page owned by this GMMU's device, so the
	// walk resolves locally (no remote request out Bottom).
	insertLocalPage := func(vAddr uint64) {
		pageTable.Insert(vm.Page{
			PID:      vm.PID(1),
			VAddr:    vAddr,
			DeviceID: deviceID,
			Valid:    true,
		})
	}

	makeTranslationReq := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: deviceID},
			ID:  sim.NewID(),
			Src: agentPort,
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
			vAddr := uint64(i) << 12
			insertLocalPage(vAddr)
			topPort.Deliver(makeTranslationReq(vAddr))
		}

		// Tick until every delivered request has been parsed into an
		// in-flight walk, so the Drain that follows must wait for real work.
		for i := 0; i < 64 &&
			len(comp.State.WalkingTranslations) < n; i++ {
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
				if _, ok := out.Payload.(vmprotocol.TranslationRsp); ok {
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
		// Every in-flight walk finished locally, and none remain, by the time
		// the async Drain ack is sent.
		require.Equal(t, n, completed)
		require.Empty(t, comp.State.WalkingTranslations)
		require.Len(t, comp.State.RemoteMemReqs, 0)
		require.Equal(t, controlStatePaused, comp.State.ControlState)
	})

	t.Run("freezes incoming translations while paused", func(t *testing.T) {
		setup1(t)

		comp.State.ControlState = controlStatePaused

		insertLocalPage(0x0)
		topPort.Deliver(makeTranslationReq(0x0))

		for range 5 {
			modelingtest.Tick(comp)
		}

		// The request is neither consumed nor turned into a walk, and no
		// translation response is produced, while paused.
		_, present0 := topPort.PeekIncoming()
		require.True(t, present0)
		require.Empty(t, comp.State.WalkingTranslations)
		_, present1 := topPort.RetrieveOutgoing()
		require.False(t, present1)
	})

	t.Run("Reset wipes in-flight state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState controlState) {
			t.Helper()

			setup1(t)

			insertLocalPage(0x0)
			topPort.Deliver(makeTranslationReq(0x0))

			// Get a walk in flight before Reset arrives.
			for i := 0; i < 64 &&
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
			require.Len(t, comp.State.RemoteMemReqs, 0)
			// RemoteMemReqs must remain a usable (non-nil) map after Reset:
			// walkMW writes to it directly, so a nil map would panic on the
			// next remote walk.
			require.NotNil(t, comp.State.RemoteMemReqs)
			require.Equal(t, controlStateEnabled, comp.State.ControlState)
		}
		t.Run("from Enabled", func(t *testing.T) { runCase(t, controlStateEnabled) })
		t.Run("from Paused", func(t *testing.T) { runCase(t, controlStatePaused) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// The component is draining and already quiescent: completePendingDrain
		// acks the Drain. Control commands are serialized with no preemption,
		// so a Reset queued behind the drain is serviced only after the Drain
		// acks.
		comp.State.ControlState = controlStateDraining
		comp.State.CurrentCmdID = 999
		comp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")
		comp.State.WalkingTranslations = nil
		comp.State.RemoteMemReqs = nil

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
