package tlb

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
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

// sleepRig runs the TLB on the serial engine and records the cycle of every tick
// and every Top response.
type sleepRig struct {
	sim    *akitasim.Simulation
	engine *timing.SerialEngine
	comp   *Comp
	top    messaging.Port
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

// tickCycle moves the test clock to the next edge and ticks c once.
func tickCycle(engine timing.Engine, c *Comp) bool {
	engine.(*testsim.Clock).Step(c.Spec.Freq)

	return modelingtest.Tick(c)
}

func newSleepRig(t *testing.T, latency, topOutgoing int) *sleepRig {
	t.Helper()

	sim := akitasim.MakeBuilder().WithTaskTracking().Build()
	spec := DefaultSpec()
	spec.Latency = latency
	spec.NumReqPerCycle = 2
	spec.Bottom.Destinations = []messaging.RemotePort{"Walker"}
	comp := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("TLB")
	port := func(in, out int) messaging.Port {
		return twowaybuffered.MakeBuilder().WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: in, OutgoingCapacity: out}).Build()
	}
	comp.BindPort("Top", port(8, topOutgoing))
	comp.BindPort("Bottom", port(4, 4))
	comp.BindPort("Control", port(4, 4))
	plugNoopConn(comp)
	sim.Engine().RegisterHandler("Driver", sleepHandler{})
	require.NoError(t, sim.Initialize())

	for i, vAddr := range []uint64{0x1000, 0x2000} {
		setUpdate(&comp.State.Sets[0], i, vm.Page{PID: 1, VAddr: vAddr, PAddr: vAddr << 8, Valid: true})
		setVisit(&comp.State.Sets[0], i)
	}

	r := &sleepRig{
		sim: sim, engine: sim.Engine().(*timing.SerialEngine),
		comp: comp, top: comp.Ports.Top, ctrl: comp.Ports.Control,
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

func (r *sleepRig) translate(cycle, vAddr uint64) {
	r.at(cycle, func() {
		r.top.Deliver(messaging.Msg{
			ID: r.sim.NewID(), Src: "Requester", Dst: r.top.AsRemote(),
			Payload: addrtransprotocol.Req{PID: 1, VAddr: vAddr, DeviceID: 1},
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

func TestTLBSleepsDuringLookupLatency(t *testing.T) {
	run := func(forceTicks bool) *sleepRig {
		r := newSleepRig(t, 100, 8)
		r.translate(5, 0x1000)
		if forceTicks {
			for cycle := uint64(6); cycle < 200; cycle++ {
				r.at(cycle, r.comp.TickLater)
			}
		}
		require.NoError(t, r.engine.Run())
		return r
	}

	lazy, eager := run(false), run(true)

	require.Len(t, lazy.sends, 1)
	require.Equal(t, eager.sends, lazy.sends, "a sleeping TLB must respond in the same cycle")
	require.Greater(t, lazy.sends[0], uint64(105))
	require.Less(t, len(lazy.ticks), 12, "the TLB must not tick while requests only travel")
	require.Greater(t, len(eager.ticks), 150)

	rsp, ok := lazy.top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(0x1000)<<8, rsp.Payload.(addrtransprotocol.Rsp).Page.PAddr)
}

// TestFirstTickAtEdgeZeroMatchesForcedTicks starts the TLB with TickAt(0).
func TestFirstTickAtEdgeZeroMatchesForcedTicks(t *testing.T) {
	for _, latency := range []int{1, 2, 7} {
		run := func(forceTicks bool) *sleepRig {
			r := newSleepRig(t, latency, 8)
			r.top.Deliver(messaging.Msg{
				ID: r.sim.NewID(), Src: "Requester", Dst: r.top.AsRemote(),
				Payload: addrtransprotocol.Req{PID: 1, VAddr: 0x1000, DeviceID: 1},
			})
			r.comp.TickAt(0)
			if forceTicks {
				for cycle := uint64(1); cycle < 30; cycle++ {
					r.comp.TickAt(timing.VTimeInPicoSec(cycle) * r.comp.Spec.Freq.Period())
				}
			}
			require.NoError(t, r.engine.Run())
			return r
		}

		lazy, eager := run(false), run(true)

		require.Len(t, lazy.sends, 1, "latency %d", latency)
		require.Equal(t, eager.sends, lazy.sends, "latency %d", latency)
	}
}

func TestTLBResumesBlockedLookupOnce(t *testing.T) {
	r := newSleepRig(t, 10, 1)
	r.translate(5, 0x1000)
	r.translate(6, 0x2000)
	r.at(200, func() {
		require.Len(t, r.sends, 1, "the second response must wait for the full port")
		_, ok := r.top.RetrieveOutgoing()
		require.True(t, ok)
	})
	r.at(220, func() {
		_, ok := r.top.RetrieveOutgoing()
		require.True(t, ok)
	})

	require.NoError(t, r.engine.Run())

	require.Len(t, r.sends, 2, "each request responds exactly once")
	require.Equal(t, uint64(201), r.sends[1])
	require.Zero(t, r.comp.State.Pipeline.Size())
	require.Zero(t, r.comp.State.BufferItems.Size())
}

func TestPausedTLBStopsPipelineTime(t *testing.T) {
	baseline := newSleepRig(t, 100, 8)
	baseline.translate(5, 0x1000)
	require.NoError(t, baseline.engine.Run())
	require.Len(t, baseline.sends, 1)

	const pausedCycles = 300
	r := newSleepRig(t, 100, 8)
	r.translate(5, 0x1000)
	r.command(40, memcontrolprotocol.CmdPause)
	r.command(40+pausedCycles, memcontrolprotocol.CmdEnable)
	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{baseline.sends[0] + pausedCycles}, r.sends)
}

func TestDrainWithTravelingRequestAcknowledgesAndResumes(t *testing.T) {
	r := newSleepRig(t, 100, 8)
	r.translate(5, 0x1000)
	r.command(30, memcontrolprotocol.CmdDrain)
	acked := false
	r.at(60, func() {
		msg, ok := r.ctrl.RetrieveOutgoing()
		require.True(t, ok, "Drain must be acknowledged")
		require.Equal(t, memcontrolprotocol.CmdDrain, msg.Payload.(memcontrolprotocol.Rsp).Command)
		acked = true
	})
	r.command(70, memcontrolprotocol.CmdEnable)

	require.NoError(t, r.engine.Run())

	require.True(t, acked)
	require.Len(t, r.sends, 1, "the frozen request must finish after Enable")
}

func TestZeroLatencyLookupCompletes(t *testing.T) {
	r := newSleepRig(t, 0, 8)
	r.translate(5, 0x2000)
	require.NoError(t, r.engine.Run())

	require.Len(t, r.sends, 1)
	rsp, ok := r.top.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, uint64(0x2000)<<8, rsp.Payload.(addrtransprotocol.Rsp).Page.PAddr)
}

func TestCheckpointKeepsPendingLookupWake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")

	ref := newSleepRig(t, 100, 8)
	ref.translate(5, 0x1000)
	stop := timing.VTimeInPicoSec(50) * ref.comp.Spec.Freq.Period()
	require.NoError(t, ref.engine.RunUntil(stop))
	require.Empty(t, ref.sends)
	require.Equal(t, 1, ref.comp.State.Pipeline.Size())
	require.NoError(t, ref.sim.SaveCheckpoint(path))
	ref.ticks = nil
	require.NoError(t, ref.engine.Run())
	require.Len(t, ref.sends, 1)

	resumed := newSleepRig(t, 100, 8)
	require.NoError(t, resumed.sim.LoadCheckpoint(path))
	require.NoError(t, resumed.engine.Run())

	require.Equal(t, ref.sends, resumed.sends)
	require.Equal(t, ref.ticks, resumed.ticks)
}
