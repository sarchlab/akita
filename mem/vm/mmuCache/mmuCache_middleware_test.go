package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestMMUCacheMiddleware(t *testing.T) {
	var (
		engine      timing.Engine
		sim         timing.Simulation
		comp        *Comp
		mw          *mmuCacheMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		spec := DefaultSpec()
		spec.NumBlocks = 4
		spec.NumLevels = 2
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{
				LowModulePort: messaging.RemotePort("LowModule"),
				UpModulePort:  messaging.RemotePort("UpModule"),
			}).
			Build("MMUCache")
		compPorts := defaultPorts("MMUCache")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		topPort = comp.Ports.Top
		bottomPort = comp.Ports.Bottom
		controlPort = comp.Ports.Control
		(&noopConn{}).BindPort(topPort)
		(&noopConn{}).BindPort(bottomPort)
		(&noopConn{}).BindPort(controlPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		mw = comp.Middlewares.Cache
	}

	t.Run("should send full latency on miss", func(t *testing.T) {
		setup1(t)

		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x2000,
			DeviceID: 3},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("UpModule"),
			Dst: topPort.AsRemote(),
		}

		topPort.Deliver(req)

		madeProgress := mw.lookup()

		require.True(t, madeProgress)

		sent, _ := bottomPort.RetrieveOutgoing()
		_, ok := sent.Payload.(vmprotocol.TranslationReq)
		sentReq := sent
		require.True(t, ok)
		require.Equal(t, uint64(200), sentReq.Payload.(vmprotocol.TranslationReq).TransLatency)
		require.Equal(t, messaging.RemotePort("LowModule"), sentReq.Dst)
		require.Equal(t, bottomPort.AsRemote(), sentReq.Src)
		require.Equal(t, vm.PID(1), sentReq.Payload.(vmprotocol.TranslationReq).PID)
		require.Equal(t, uint64(0x2000), sentReq.Payload.(vmprotocol.TranslationReq).VAddr)
		require.Equal(t, uint64(3), sentReq.Payload.(vmprotocol.TranslationReq).DeviceID)
		_, present0 := topPort.PeekIncoming()
		require.False(t, present0)
	})

	t.Run("should reduce latency on upper-level hit", func(t *testing.T) {
		setup1(t)

		req := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x3000,
			DeviceID: 2},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("UpModule"),
			Dst: topPort.AsRemote(),
		}

		// Compute seg and wayID for level 1
		spec := comp.Spec
		seg := segForLevelSpec(spec, 1, req.Payload.(vmprotocol.TranslationReq).VAddr)
		wayID := setIDForSegSpec(spec, seg)

		// Update the set in state
		next := &comp.State
		setUpdate(&next.Table[1], wayID, req.Payload.(vmprotocol.TranslationReq).PID, seg)

		topPort.Deliver(req)

		madeProgress := mw.lookup()

		require.True(t, madeProgress)

		sent, _ := bottomPort.RetrieveOutgoing()
		_, ok := sent.Payload.(vmprotocol.TranslationReq)
		sentReq := sent
		require.True(t, ok)
		require.Equal(t, uint64(100), sentReq.Payload.(vmprotocol.TranslationReq).TransLatency)
	})

	t.Run("should forward response and update cache", func(t *testing.T) {
		setup1(t)

		page := vm.Page{
			PID:   1,
			VAddr: 0x4000,
			PAddr: 0x5000,
			Valid: true,
		}
		rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: page,
		},
			ID:    sim.NewID(),
			Src:   messaging.RemotePort("LowModule"),
			Dst:   bottomPort.AsRemote(),
			RspTo: sim.NewID(),
		}

		bottomPort.Deliver(rsp)

		// Mark the response's request as outstanding, as a real forward would.
		comp.State.OutstandingBottomReqs[rsp.RspTo] = true

		madeProgress := mw.handleRsp(rsp)

		spec := comp.Spec
		next := &comp.State
		require.True(t, madeProgress)

		sent, _ := topPort.RetrieveOutgoing()
		_, ok := sent.Payload.(vmprotocol.TranslationRsp)
		sentRsp := sent
		require.True(t, ok)
		require.Equal(t, messaging.RemotePort("UpModule"), sentRsp.Dst)
		require.Equal(t, topPort.AsRemote(), sentRsp.Src)
		require.Equal(t, page, sentRsp.Payload.(vmprotocol.TranslationRsp).Page)

		for level := 0; level < spec.NumLevels; level++ {
			seg := segForLevelSpec(spec, level, page.VAddr)
			_, found := setLookup(&next.Table[level], page.PID, seg)
			require.True(t, found)
		}
	})

	t.Run("drops the addressed page's segments while a sibling page survives", func(t *testing.T) {
		setup1(t)

		pid := vm.PID(1)
		spec := comp.Spec

		// dropAddr and keepAddr share their upper-level (level-1) segment
		// but differ at level 0. A page-walk cache stores per-level VPN
		// segments, so invalidating dropAddr removes its level-0 segment
		// (and the shared upper-level segment), while keepAddr's distinct
		// level-0 segment must survive.
		dropAddr := uint64(0x6000)
		keepAddr := uint64(0x9000)
		require.NotEqual(t, segForLevelSpec(spec, 0, keepAddr), segForLevelSpec(spec, 0, dropAddr))
		require.Equal(t, segForLevelSpec(spec, 1, keepAddr), segForLevelSpec(spec, 1, dropAddr))

		next := &comp.State
		next.CurrentState = mmuCacheStatePause

		for level := 0; level < spec.NumLevels; level++ {
			dropSeg := segForLevelSpec(spec, level, dropAddr)
			keepSeg := segForLevelSpec(spec, level, keepAddr)
			setUpdate(&next.Table[level], setIDForSegSpec(spec, dropSeg), pid, dropSeg)
			setVisit(&next.Table[level], setIDForSegSpec(spec, dropSeg))
			setUpdate(&next.Table[level], setIDForSegSpec(spec, keepSeg), pid, keepSeg)
			setVisit(&next.Table[level], setIDForSegSpec(spec, keepSeg))
		}

		ctrl := comp.Middlewares.Ctrl
		req := messaging.Msg{Payload: memcontrolprotocol.Req{
			Command:   memcontrolprotocol.CmdInvalidate,
			Addresses: []uint64{dropAddr},
		},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: controlPort.AsRemote(),
		}

		controlPort.Deliver(req)

		require.True(t, ctrl.handleIncomingCommands())

		// The dropped address's segment is gone at every level it cached.
		for level := 0; level < spec.NumLevels; level++ {
			dropSeg := segForLevelSpec(spec, level, dropAddr)
			_, dropFound := setLookup(&next.Table[level], pid, dropSeg)
			require.False(t, dropFound)
		}
		// keepAddr's distinct level-0 segment survives.
		keepSeg0 := segForLevelSpec(spec, 0, keepAddr)
		_, keepFound := setLookup(&next.Table[0], pid, keepSeg0)
		require.True(t, keepFound)

		sentRspValue, _ := controlPort.RetrieveOutgoing()

		sentRsp := sentRspValue
		require.Equal(t, memcontrolprotocol.CmdInvalidate, sentRsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, sentRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, messaging.RemotePort("Requester"), sentRsp.Dst)
		require.Equal(t, controlPort.AsRemote(), sentRsp.Src)
	})
}

func segForLevelSpec(spec Spec, level int, vAddr uint64) uint64 {
	vpn := vAddr >> spec.Log2PageSize
	levelWidth := (64 - spec.Log2PageSize) / uint64(spec.NumLevels)
	return (vpn >> (uint64(level) * levelWidth)) & ((1 << levelWidth) - 1)
}

func setIDForSegSpec(spec Spec, seg uint64) int {
	return int(seg % uint64(spec.NumBlocks))
}
