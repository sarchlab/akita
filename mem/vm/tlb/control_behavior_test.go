package tlb

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

// This file holds Layer-2 control-behavior tests for the TLB: it asserts the
// actual behavior the universal verbs promise (Drain quiescence, Pause freeze,
// Reset from every state), beyond the protocol-surface checks in
// control_contract_test.go.
//
// The TLB is downstream-dependent: a lookup miss creates an MSHR entry and
// forwards a request out the "Bottom" port; the request does not complete
// until a matching addrtransprotocol.Rsp is fed back on "Bottom". The Drain test
// below drives real completion through that handshake.
func TestTLBControlBehavior(t *testing.T) {
	var (
		engine      timing.Engine
		sim         *testsim.Simulation
		tlbComp     *Comp
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
		remotePort  = messaging.RemotePort("MMU")
	)

	build := func() {
		spec := DefaultSpec()

		spec.Bottom.Destinations = []messaging.RemotePort{remotePort}
		tlbComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("TLB")
		tlbCompPorts := defaultPorts(sim, "TLB")
		tlbComp.BindPort("Top", tlbCompPorts.Top)
		tlbComp.BindPort("Bottom", tlbCompPorts.Bottom)
		tlbComp.BindPort("Control", tlbCompPorts.Control)

		topPort = tlbComp.Ports.Top
		bottomPort = tlbComp.Ports.Bottom
		controlPort = tlbComp.Ports.Control
		for _, p := range []messaging.Port{topPort, bottomPort, controlPort} {
			(&ccNoopConn{}).BindPort(p)
		}
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	makeLookup := func(vAddr uint64) messaging.Msg {
		req := messaging.Msg{Payload: addrtransprotocol.Req{
			PID:      1,
			VAddr:    vAddr,
			DeviceID: 1},
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
			Dst: controlPort.AsRemote(),
		}

		return req
	}

	// makeBottomRsp builds the addrtransprotocol.Rsp that retires the MSHR entry
	// for the forwarded bottom request. parseBottom matches the response to an
	// MSHR entry by the resolved page's PID/VAddr, so those must equal the
	// request's PID/VAddr; RspTo is set to the forwarded request's ID to mirror
	// the real downstream handshake.
	makeBottomRsp := func(req messaging.Msg) messaging.Msg {
		page := vm.Page{
			PID:   req.Payload.(addrtransprotocol.Req).PID,
			VAddr: req.Payload.(addrtransprotocol.Req).VAddr,
			PAddr: req.Payload.(addrtransprotocol.Req).VAddr + 0x10000,
			Valid: true,
		}
		rsp := messaging.Msg{Payload: addrtransprotocol.Rsp{Page: page},
			ID:    sim.NewID(),
			Src:   remotePort,
			Dst:   bottomPort.AsRemote(),
			RspTo: req.ID,
		}

		return rsp
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		build()
	}

	t.Run("drains all in-flight misses before acking Drain", func(t *testing.T) {
		setup1(t)

		const n = 2

		// Deliver N distinct-VAddr lookups that all miss (fresh TLB, distinct
		// pages so each gets its own MSHR entry).
		lookups := []messaging.Msg{
			makeLookup(0x0),
			makeLookup(0x1000),
		}
		for _, req := range lookups {
			topPort.Deliver(req)
		}

		// Tick until both misses are in flight: 2 MSHR entries created and 2
		// requests forwarded out Bottom. Capture the forwarded request IDs.
		var bottomReqs []messaging.Msg
		for i := 0; i < 64 &&
			(len(tlbComp.State.MSHREntries) < n || len(bottomReqs) < n); i++ {
			modelingtest.Tick(tlbComp)
			for {
				out, ok := bottomPort.RetrieveOutgoing()
				if !ok {
					break
				}
				bottomReqs = append(bottomReqs, out)
			}
		}

		// In-flight is genuinely present before Drain, so the test has teeth.
		require.Len(t, tlbComp.State.MSHREntries, n)
		require.Len(t, bottomReqs, n)
		require.False(t, mshrIsEmpty(tlbComp.State.MSHREntries))

		// Issue Drain.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		controlPort.Deliver(drain)

		// Negative phase: without feeding responses, Drain must NOT complete.
		var drainRsp messaging.Msg
		drainFound := false
		for range 5 {
			modelingtest.Tick(tlbComp)
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainRsp = out

					drainFound = true
				}
			}
		}

		require.False(t, drainFound)
		require.Equal(t, tlbStateDrain, tlbComp.State.TLBState)
		require.False(t, mshrIsEmpty(tlbComp.State.MSHREntries))

		// Positive phase: feed a matching bottom response for each forwarded
		// request, then tick while counting completed responses on Top and
		// watching Control for the Drain ack.
		for _, req := range bottomReqs {
			bottomPort.Deliver(makeBottomRsp(req))
		}

		completed := 0
		for i := 0; i < 256 && !drainFound; i++ {
			modelingtest.Tick(tlbComp)
			for {
				out, ok := topPort.RetrieveOutgoing()
				if !ok {
					break
				}
				if _, ok := out.Payload.(addrtransprotocol.Rsp); ok {
					completed++
				}
			}
			if out, ok := controlPort.RetrieveOutgoing(); ok {
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
		// Every in-flight miss finishes cleanly before the async Drain ack:
		// handleDrain stays draining until HasRespondingMSHR clears, so the
		// translation response that retires the final MSHR entry reaches Top
		// before the component transitions to Paused.
		require.Equal(t, n, completed)
		require.True(t, mshrIsEmpty(tlbComp.State.MSHREntries))
		require.Equal(t, tlbStatePause, tlbComp.State.TLBState)
	})

	t.Run("does not admit new Top traffic while draining", func(t *testing.T) {
		setup1(t)

		// Get a miss in flight so Drain has something to wait for.
		topPort.Deliver(makeLookup(0x0))
		var bottomReq messaging.Msg
		got := false
		for i := 0; i < 64 && !got; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := bottomPort.RetrieveOutgoing(); ok {
				_, got = out.Payload.(addrtransprotocol.Req)
				bottomReq = out
			}
		}
		require.True(t, got)
		require.False(t, mshrIsEmpty(tlbComp.State.MSHREntries))

		// Drain, and let it take effect.
		drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
		controlPort.Deliver(drain)
		for i := 0; i < 8 && tlbComp.State.TLBState != tlbStateDrain; i++ {
			modelingtest.Tick(tlbComp)
		}
		require.Equal(t, tlbStateDrain, tlbComp.State.TLBState)

		// A lookup delivered while draining must not be admitted: no new Bottom
		// forward, it stays queued on Top, and Drain must not ack.
		topPort.Deliver(makeLookup(0x5000))
		for range 8 {
			modelingtest.Tick(tlbComp)
			_, present0 := bottomPort.RetrieveOutgoing()
			require.False(t, present0)
			_, present1 := controlPort.RetrieveOutgoing()
			require.False(t, present1)
		}
		require.Equal(t, tlbStateDrain, tlbComp.State.TLBState)
		_, present2 := topPort.PeekIncoming()
		require.True(t, present2) // late lookup still queued

		// Completing the in-flight miss lets Drain finish; the late lookup is
		// still queued for after Enable.
		bottomPort.Deliver(makeBottomRsp(bottomReq))
		drainFound := false
		for i := 0; i < 256 && !drainFound; i++ {
			modelingtest.Tick(tlbComp)
			for {
				if _, ok := topPort.RetrieveOutgoing(); !ok {
					break
				}
			}
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if rsp, ok := out.Payload.(memcontrolprotocol.Rsp); ok && rsp.
					Command == memcontrolprotocol.CmdDrain {
					drainFound = true
				}
			}
		}
		require.True(t, drainFound)
		require.Equal(t, tlbStatePause, tlbComp.State.TLBState)
		_, present3 := topPort.PeekIncoming()
		require.True(t, present3) // late lookup survived
	})

	t.Run("drops a stale bottom translation that arrives after Reset", func(t *testing.T) {
		setup1(t)

		// First lookup misses and forwards a bottom request (MSHR entry A).
		topPort.Deliver(makeLookup(0x100))
		var reqA messaging.Msg
		gotA := false
		for i := 0; i < 64 && !gotA; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := bottomPort.RetrieveOutgoing(); ok {
				_, gotA = out.Payload.(addrtransprotocol.Req)
				reqA = out
			}
		}
		require.True(t, gotA)

		// Reset discards the outstanding walk and the TLB contents.
		rst := makeCtrlReq(memcontrolprotocol.CmdReset)
		controlPort.Deliver(rst)
		acked := false
		for i := 0; i < 64 && !acked; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := controlPort.RetrieveOutgoing(); ok {
				if r, ok := out.Payload.(memcontrolprotocol.Rsp); ok && r.
					Command == memcontrolprotocol.CmdReset {
					acked = true
				}
			}
		}
		require.True(t, acked)

		// A new lookup for the SAME address misses and forwards a fresh bottom
		// request (MSHR entry B) with a different ID.
		topPort.Deliver(makeLookup(0x100))
		var reqB messaging.Msg
		gotB := false
		for i := 0; i < 64 && !gotB; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := bottomPort.RetrieveOutgoing(); ok {
				_, gotB = out.Payload.(addrtransprotocol.Req)
				reqB = out
			}
		}
		require.True(t, gotB)
		require.NotEqual(t, reqA.ID, reqB.ID)

		// The stale pre-reset response (for request A) arrives. It must be
		// dropped: no Top response, and the new request's MSHR entry survives.
		bottomPort.Deliver(makeBottomRsp(reqA))
		for range 8 {
			modelingtest.Tick(tlbComp)
			_, present4 := topPort.RetrieveOutgoing()
			require.False(t, present4)
		}
		require.True(t, mshrIsEntryPresent(
			tlbComp.State.MSHREntries,
			reqB.Payload.(addrtransprotocol.Req).PID,
			reqB.Payload.(addrtransprotocol.Req).VAddr))

		// The legitimate response (for request B) is accepted and answered.
		bottomPort.Deliver(makeBottomRsp(reqB))
		answered := false
		for i := 0; i < 64 && !answered; i++ {
			modelingtest.Tick(tlbComp)
			if out, ok := topPort.RetrieveOutgoing(); ok {
				if _, ok := out.Payload.(addrtransprotocol.Rsp); ok {
					answered = true
				}
			}
		}
		require.True(t, answered)
	})

	t.Run("freezes incoming traffic while paused", func(t *testing.T) {
		setup1(t)

		tlbComp.State.TLBState = tlbStatePause
		topPort.Deliver(makeLookup(0x0))

		for range 5 {
			modelingtest.Tick(tlbComp)
		}

		// The request is neither consumed nor turned into work, and nothing is
		// forwarded out Bottom, while paused.
		_, present5 := topPort.PeekIncoming()
		require.True(t, present5)
		require.True(t, mshrIsEmpty(tlbComp.State.MSHREntries))
		_, present6 := bottomPort.RetrieveOutgoing()
		require.False(t, present6)
	})

	t.Run("Reset wipes in-flight MSHR state from any control state", func(t *testing.T) {
		runCase := func(t *testing.T, startState string) {
			t.Helper()

			setup1(t)

			// Get one miss in flight: an MSHR entry and a forwarded bottom req.
			topPort.Deliver(makeLookup(0x0))
			for i := 0; i < 64 && mshrIsEmpty(tlbComp.State.MSHREntries); i++ {
				modelingtest.Tick(tlbComp)
			}
			require.False(t, mshrIsEmpty(tlbComp.State.MSHREntries))

			tlbComp.State.TLBState = startState

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			controlPort.Deliver(reset)

			var rsp messaging.Msg
			found := false
			for i := 0; i < 64 && !found; i++ {
				modelingtest.Tick(tlbComp)
				if out, ok := controlPort.RetrieveOutgoing(); ok {
					_, found = out.Payload.(memcontrolprotocol.Rsp)
					rsp = out
				}
			}

			require.True(t, found)
			require.Equal(t, memcontrolprotocol.CmdReset, rsp.Payload.(memcontrolprotocol.Rsp).Command)
			require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
			require.Equal(t, reset.ID, rsp.RspTo)
			// Reset is a hard reset: the in-flight MSHR entry is discarded
			// (handleReset clears MSHREntries) and the TLB returns to Enabled.
			require.True(t, mshrIsEmpty(tlbComp.State.MSHREntries))
			require.Equal(t, tlbStateEnable, tlbComp.State.TLBState)
		}
		t.Run("from Enable", func(t *testing.T) { runCase(t, tlbStateEnable) })
		t.Run("from Pause", func(t *testing.T) { runCase(t, tlbStatePause) })
	})

	t.Run("completes a pending Drain before servicing a queued Reset", func(t *testing.T) {
		setup1(t)

		// Draining with the drain ack pending and no in-flight work: the data
		// path flips the state to pause and completePendingDrain acks the
		// Drain. Control commands are serialized with no preemption, so a Reset
		// queued behind the drain is serviced only after the Drain acks.
		tlbComp.State.TLBState = tlbStateDrain
		tlbComp.State.PendingDrainRsp = true
		tlbComp.State.CurrentCmdID = 999
		tlbComp.State.CurrentCmdSrc = messaging.RemotePort("Drainer")

		reset := makeCtrlReq(memcontrolprotocol.CmdReset)
		controlPort.Deliver(reset)

		var rsps []messaging.Msg
		for range 32 {
			modelingtest.Tick(tlbComp)
			for {
				out, ok := controlPort.RetrieveOutgoing()
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
		require.Equal(t, tlbStateEnable, tlbComp.State.TLBState)
	})
}
