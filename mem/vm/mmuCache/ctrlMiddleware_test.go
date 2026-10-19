package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestMMUCacheCtrlMiddleware(t *testing.T) {
	var (
		engine      timing.Engine
		sim         timing.Simulation
		comp        *Comp
		ctrl        *ctrlMiddleware
		topPort     messaging.Port
		bottomPort  messaging.Port
		controlPort messaging.Port
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		spec := DefaultSpec()
		spec.NumBlocks = 1
		spec.NumLevels = 5
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		comp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			Build("MMUCache")
		compPorts := defaultPorts("MMUCache")
		comp.BindPort("Top", compPorts.Top)
		comp.BindPort("Bottom", compPorts.Bottom)
		comp.BindPort("Control", compPorts.Control)

		comp.State.CurrentState = mmuCacheStatePause

		topPort = comp.Ports.Top
		bottomPort = comp.Ports.Bottom
		controlPort = comp.Ports.Control
		(&noopConn{}).BindPort(topPort)
		(&noopConn{}).BindPort(bottomPort)
		(&noopConn{}).BindPort(controlPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		ctrl = comp.Middlewares.Ctrl
	}

	t.Run("should do nothing when no control message", func(t *testing.T) {
		setup1(t)

		madeProgress := ctrl.Handle(modelingtest.TickEvent(comp))

		require.False(t, madeProgress)
	})

	t.Run("should restart and drain ports", func(t *testing.T) {
		setup1(t)

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
		}

		topMsg := messaging.Msg{Payload: vmprotocol.TranslationReq{
			PID:      1,
			VAddr:    0x1000,
			DeviceID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: topPort.AsRemote(),
		}

		topPort.Deliver(topMsg)

		bottomMsg := messaging.Msg{Payload: vmprotocol.TranslationRsp{
			Page: vm.Page{},
		},
			ID:    sim.NewID(),
			Src:   messaging.RemotePort("LowModule"),
			Dst:   bottomPort.AsRemote(),
			RspTo: sim.NewID(),
		}

		bottomPort.Deliver(bottomMsg)

		madeProgress := ctrl.handleReset(req)

		next := &comp.State
		require.True(t, madeProgress)
		require.Equal(t, mmuCacheStateEnable, next.CurrentState)
		_, present0 := topPort.PeekIncoming()
		require.False(t, present0)
		_, present1 := bottomPort.PeekIncoming()
		require.False(t, present1)
		rsp, _ := controlPort.RetrieveOutgoing()
		_, ok := rsp.Payload.(memcontrolprotocol.Rsp)
		ctrlRsp := rsp
		require.True(t, ok)
		require.Equal(t, memcontrolprotocol.CmdReset, ctrlRsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, ctrlRsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, messaging.RemotePort("Requester"), ctrlRsp.Dst)
		require.Equal(t, controlPort.AsRemote(), ctrlRsp.Src)
	})

	t.Run("should reject Flush as unsupported", func(t *testing.T) {
		setup1(t)

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: controlPort.AsRemote(),
		}

		controlPort.Deliver(req)

		require.True(t, ctrl.handleIncomingCommands())

		rspValue, _ := controlPort.RetrieveOutgoing()

		rsp := rspValue
		require.Equal(t, memcontrolprotocol.CmdFlush, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.False(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, memcontrolprotocol.ErrUnsupported, rsp.Payload.(memcontrolprotocol.Rsp).Error)
	})

	t.Run("should invalidate cached segments when paused", func(t *testing.T) {
		setup1(t)

		spec := comp.Spec
		next := &comp.State
		next.CurrentState = mmuCacheStatePause

		pid := vm.PID(1)
		vAddr := uint64(0x6000)
		seg := segForLevelSpec(spec, 0, vAddr)
		wayID := setIDForSegSpec(spec, seg)
		setUpdate(&next.Table[0], wayID, pid, seg)
		setVisit(&next.Table[0], wayID)

		_, found := setLookup(&next.Table[0], pid, seg)
		require.True(t, found)

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: controlPort.AsRemote(),
		}

		controlPort.Deliver(req)

		require.True(t, ctrl.handleIncomingCommands())

		_, found = setLookup(&next.Table[0], pid, seg)
		require.False(t, found)

		rspValue, _ := controlPort.RetrieveOutgoing()

		rsp := rspValue
		require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
	})

	t.Run("should reject Invalidate while enabled", func(t *testing.T) {
		setup1(t)

		next := &comp.State
		next.CurrentState = mmuCacheStateEnable

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: controlPort.AsRemote(),
		}

		controlPort.Deliver(req)

		require.True(t, ctrl.handleIncomingCommands())

		rspValue, _ := controlPort.RetrieveOutgoing()

		rsp := rspValue
		require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.False(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
		require.Equal(t, memcontrolprotocol.ErrMustBePausedOrDrained, rsp.Payload.(memcontrolprotocol.Rsp).Error)
	})

	t.Run("invalidates only entries matching the PID filter", func(t *testing.T) {
		setup1(t)

		// A two-way single-level cache so two PIDs can coexist in
		// distinct ways and the filter's selectivity is observable.
		spec := DefaultSpec()
		spec.NumBlocks = 2
		spec.NumLevels = 1
		spec.PageSize = 4096
		spec.Log2PageSize = 12
		spec.NumReqPerCycle = 4
		spec.LatencyPerLevel = 100

		reg2 := akitasim.MakeBuilder().Build()
		comp2 := modeling.MakeBuilder[*Comp](spec).
			WithSimulation(reg2).
			Build("MMUCache2")
		comp2Ports := defaultPorts("MMUCache2")
		comp2.BindPort("Top", comp2Ports.Top)
		comp2.BindPort("Bottom", comp2Ports.Bottom)
		comp2.BindPort("Control", comp2Ports.Control)

		control2 := comp2.Ports.Control
		for _, p := range []messaging.Port{
			comp2.Ports.Top, comp2.Ports.Bottom, comp2.Ports.Control,
		} {
			(&noopConn{}).BindPort(p)
		}
		if err := reg2.Initialize(); err != nil {
			panic(err)
		}

		ctrl2 := comp2.Middlewares.Ctrl

		next := &comp2.State
		next.CurrentState = mmuCacheStatePause

		segA := uint64(0xa)
		segB := uint64(0xb)
		setUpdate(&next.Table[0], 0, vm.PID(1), segA)
		setVisit(&next.Table[0], 0)
		setUpdate(&next.Table[0], 1, vm.PID(2), segB)
		setVisit(&next.Table[0], 1)

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdInvalidate, PID: 1},
			ID:  sim.NewID(),
			Src: messaging.RemotePort("Requester"),
			Dst: control2.AsRemote(),
		}

		control2.Deliver(req)

		require.True(t, ctrl2.handleIncomingCommands())

		// Only the PID-1 entry is dropped; the PID-2 entry survives.
		_, foundA := setLookup(&next.Table[0], vm.PID(1), segA)
		require.False(t, foundA)
		_, foundB := setLookup(&next.Table[0], vm.PID(2), segB)
		require.True(t, foundB)

		rspValue, _ := control2.RetrieveOutgoing()

		rsp := rspValue
		require.Equal(t, memcontrolprotocol.CmdInvalidate, rsp.Payload.(memcontrolprotocol.Rsp).Command)
		require.True(t, rsp.Payload.(memcontrolprotocol.Rsp).Success)
	})

	t.Run("should handle control pause", func(t *testing.T) {
		setup1(t)

		spec := comp.Spec
		comp.State = state{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		}

		msg := messaging.Msg{Payload: memcontrolprotocol.Req{
			Command: memcontrolprotocol.CmdPause,
		},
			ID:           sim.NewID(),
			Src:          messaging.RemotePort("Requester"),
			Dst:          controlPort.AsRemote(),
			TrafficBytes: 4,
		}

		controlPort.Deliver(msg)

		madeProgress := ctrl.handleIncomingCommands()

		next := &comp.State
		require.True(t, madeProgress)
		require.Equal(t, mmuCacheStatePause, next.CurrentState)
	})
}
