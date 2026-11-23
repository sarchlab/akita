package writethrough

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type sleepEvent struct {
	timing.EventBase
	action func()
}

type sleepHandler struct{}

func (sleepHandler) Handle(e timing.Event) { e.(sleepEvent).action() }

// sleepRig runs the cache on the serial engine and records the cycle of every
// tick and every Top response.
type sleepRig struct {
	t      *testing.T
	sim    *akitasim.Simulation
	engine *timing.SerialEngine
	comp   *Comp
	top    messaging.Port
	bottom messaging.Port
	ctrl   messaging.Port
	ticks  []uint64
	sends  []uint64
}

func (r *sleepRig) Func(ctx hooking.HookCtx) {
	cycle := r.comp.Spec.Freq.Cycle(r.engine.CurrentTime())
	switch ctx.Pos {
	case timing.HookPosBeforeEvent:
		if e, ok := ctx.Item.(ticking.Event); ok && e.HandlerID() == r.comp.Name() {
			r.ticks = append(r.ticks, cycle)
		}
	case messaging.HookPosPortMsgSend:
		r.sends = append(r.sends, cycle)
	}
}

func sleepSpec() Spec {
	spec := DefaultSpec()
	spec.Bottom.Destinations = []messaging.RemotePort{"Lower"}
	spec.NumSets, spec.NumWays = 16, 2
	spec.BankLatency = 120
	spec.DirLatency = 3

	return spec
}

func newSleepRig(t *testing.T, spec Spec, topOutgoing int) *sleepRig {
	t.Helper()

	sim := akitasim.MakeBuilder().WithTaskTracking().Build()
	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(Resources{Storage: mem.NewStorage(1 * mem.MB)}).
		Build("Cache")
	port := func(in, out int) messaging.Port {
		return twowaybuffered.MakeBuilder().WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: in, OutgoingCapacity: out}).Build()
	}
	comp.BindPort("Top", port(8, topOutgoing))
	comp.BindPort("Bottom", port(8, 8))
	comp.BindPort("Control", port(4, 4))
	for _, p := range []messaging.Port{comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control} {
		(&ccNoopConn{}).BindPort(p)
	}
	sim.Engine().RegisterHandler("Driver", sleepHandler{})
	require.NoError(t, sim.Initialize())

	r := &sleepRig{
		t: t, sim: sim, engine: sim.Engine().(*timing.SerialEngine),
		comp: comp, top: comp.Ports.Top, bottom: comp.Ports.Bottom, ctrl: comp.Ports.Control,
	}
	r.engine.AcceptHook(r)
	r.top.AcceptHook(r)

	return r
}

// at runs action after the components of the given cycle, as a connection does.
func (r *sleepRig) at(cycle uint64, action func()) {
	base := timing.MakeEventBase(r.sim.NewID(),
		timing.VTimeInPicoSec(cycle)*r.comp.Spec.Freq.Period(), "Driver")
	base.Secondary = true
	r.engine.Schedule(sleepEvent{EventBase: base, action: action})
}

func (r *sleepRig) read(cycle, address uint64) {
	r.at(cycle, func() {
		r.top.Deliver(messaging.Msg{
			ID: r.sim.NewID(), Src: "Agent", Dst: r.top.AsRemote(), TrafficBytes: 12,
			Payload: memprotocol.ReadReq{Address: address, AccessByteSize: 64},
		})
	})
}

// fill answers the oldest outstanding Bottom read with a line of value.
func (r *sleepRig) fill(cycle uint64, value byte) {
	r.at(cycle, func() {
		req, ok := r.bottom.RetrieveOutgoing()
		require.True(r.t, ok, "the miss must have reached Bottom")
		data := make([]byte, 64)
		for i := range data {
			data[i] = value
		}
		r.bottom.Deliver(messaging.Msg{
			ID: r.sim.NewID(), Src: "Lower", Dst: r.bottom.AsRemote(), RspTo: req.ID,
			TrafficBytes: 68, Payload: memprotocol.DataReadyRsp{Data: data},
		})
	})
}

func (r *sleepRig) command(cycle uint64, cmd memcontrolprotocol.Command) {
	r.at(cycle, func() {
		r.ctrl.Deliver(messaging.Msg{
			ID: r.sim.NewID(), Src: "Ctrl", Dst: r.ctrl.AsRemote(),
			Payload: memcontrolprotocol.Req{Command: cmd},
		})
	})
}

func requireLine(t *testing.T, msg messaging.Msg, value byte) {
	t.Helper()
	data := msg.Payload.(memprotocol.DataReadyRsp).Data
	require.Len(t, data, 64)
	for _, b := range data {
		require.Equal(t, value, b)
	}
}

func TestCacheSleepsDuringBankTraversal(t *testing.T) {
	run := func(forceTicks bool) *sleepRig {
		r := newSleepRig(t, sleepSpec(), 8)
		r.read(5, 0x40)
		r.fill(30, 7)
		if forceTicks {
			for cycle := uint64(6); cycle < 300; cycle++ {
				r.at(cycle, r.comp.TickLater)
			}
		}
		require.NoError(t, r.engine.Run())
		return r
	}

	lazy, eager := run(false), run(true)

	require.Len(t, lazy.sends, 1)
	require.Equal(t, eager.sends, lazy.sends, "a sleeping cache must respond in the same cycle")
	require.Greater(t, lazy.sends[0], uint64(150))
	require.Less(t, len(lazy.ticks), 30, "the cache must not tick while items only travel")
	require.Greater(t, len(eager.ticks), 250)

	rsp, ok := lazy.top.RetrieveOutgoing()
	require.True(t, ok)
	requireLine(t, rsp, 7)
}

func TestCacheResumesBlockedCompletionOnce(t *testing.T) {
	r := newSleepRig(t, sleepSpec(), 1)
	r.read(5, 0x40)
	r.read(6, 0x80)
	r.fill(30, 1)
	r.fill(31, 2)
	r.at(400, func() {
		require.Len(t, r.sends, 1, "the second response must wait for the full port")
		rsp, ok := r.top.RetrieveOutgoing()
		require.True(t, ok)
		requireLine(t, rsp, 1)
	})
	r.at(420, func() {
		rsp, ok := r.top.RetrieveOutgoing()
		require.True(t, ok)
		requireLine(t, rsp, 2)
	})

	require.NoError(t, r.engine.Run())

	require.Len(t, r.sends, 2, "each request responds exactly once")
	require.Equal(t, uint64(401), r.sends[1])
	blocked := 0
	for _, tick := range r.ticks {
		if tick > r.sends[0]+2 && tick <= 400 {
			blocked++
		}
	}
	require.Zero(t, blocked, "a blocked cache must wait for the port notification")
}

func TestPausedCacheStopsPipelineTime(t *testing.T) {
	baseline := newSleepRig(t, sleepSpec(), 8)
	baseline.read(5, 0x40)
	baseline.fill(30, 1)
	require.NoError(t, baseline.engine.Run())
	require.Len(t, baseline.sends, 1)

	const pausedCycles = 300
	r := newSleepRig(t, sleepSpec(), 8)
	r.read(5, 0x40)
	r.fill(30, 1)
	r.command(60, memcontrolprotocol.CmdPause)
	r.command(60+pausedCycles, memcontrolprotocol.CmdEnable)
	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{baseline.sends[0] + pausedCycles}, r.sends)
}

func TestResetDiscardsPipelineWorkAndKeepsServing(t *testing.T) {
	spec := sleepSpec()
	spec.DirLatency = 20
	r := newSleepRig(t, spec, 8)
	r.read(5, 0x40)
	r.read(6, 0x80)
	r.command(10, memcontrolprotocol.CmdReset)
	r.at(20, func() {
		require.Zero(t, r.comp.State.DirPipeline.Size())
		require.Empty(t, r.comp.State.Transactions)
		for {
			if _, ok := r.bottom.RetrieveOutgoing(); !ok {
				return
			}
		}
	})
	r.read(60, 0xC0)
	r.fill(100, 3)

	require.NoError(t, r.engine.Run())

	require.Len(t, r.sends, 1, "only the request after Reset responds")
	rsp, ok := r.top.RetrieveOutgoing()
	require.True(t, ok)
	requireLine(t, rsp, 3)
}

func TestZeroLatencyStagesComplete(t *testing.T) {
	for name, modify := range map[string]func(*Spec){
		"directory": func(s *Spec) { s.DirLatency = 0 },
		"bank":      func(s *Spec) { s.BankLatency = 0 },
		"both":      func(s *Spec) { s.DirLatency, s.BankLatency = 0, 0 },
	} {
		t.Run(name, func(t *testing.T) {
			spec := sleepSpec()
			modify(&spec)
			r := newSleepRig(t, spec, 8)
			r.read(5, 0x40)
			r.fill(40, 5)
			require.NoError(t, r.engine.Run())

			require.Len(t, r.sends, 1)
			rsp, ok := r.top.RetrieveOutgoing()
			require.True(t, ok)
			requireLine(t, rsp, 5)
		})
	}
}

func TestCheckpointKeepsPendingBankWake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")

	ref := newSleepRig(t, sleepSpec(), 8)
	ref.read(5, 0x40)
	ref.fill(30, 9)
	stop := timing.VTimeInPicoSec(80) * ref.comp.Spec.Freq.Period()
	require.NoError(t, ref.engine.RunUntil(stop))
	require.Empty(t, ref.sends)
	require.NotZero(t, ref.comp.State.BankPipelines[0].Size())
	require.NoError(t, ref.sim.SaveCheckpoint(path))
	ref.ticks = nil
	require.NoError(t, ref.engine.Run())
	require.Len(t, ref.sends, 1)

	resumed := newSleepRig(t, sleepSpec(), 8)
	require.NoError(t, resumed.sim.LoadCheckpoint(path))
	require.NoError(t, resumed.engine.Run())

	require.Equal(t, ref.sends, resumed.sends)
	require.Equal(t, ref.ticks, resumed.ticks)
	rsp, ok := resumed.top.RetrieveOutgoing()
	require.True(t, ok)
	requireLine(t, rsp, 9)
}
