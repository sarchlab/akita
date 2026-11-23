package switches

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type tickCounter struct {
	name  string
	ticks int
}

func (c *tickCounter) Func(ctx hooking.HookCtx) {
	if ctx.Pos != timing.HookPosBeforeEvent {
		return
	}
	if e, ok := ctx.Item.(ticking.Event); ok && e.HandlerID() == c.name {
		c.ticks++
	}
}

// sleepSwitch builds a one-input switch with real ports on a serial engine.
func sleepSwitch(t *testing.T, latency int) (*Comp, []*twowaybuffered.Port, *timing.SerialEngine) {
	t.Helper()
	s := sim.MakeBuilder().Build()
	t.Cleanup(func() { require.NoError(t, s.Terminate()) })
	links := []Link{
		{Remote: "Remote0", NumInputChannel: 1, NumOutputChannel: 1, Latency: latency},
		{Remote: "Remote1", NumInputChannel: 1, NumOutputChannel: 1, Latency: latency},
	}
	wiring := &testWiring{links: links, table: fixedRoute("Switch.Links[1]")}
	sw := modeling.MakeBuilder[*Comp](DefaultSpec()).WithSimulation(s).
		WithResources(Resources{Wiring: wiring}).Build("Switch")
	ports := make([]*twowaybuffered.Port, len(links))
	for i := range ports {
		ports[i] = twowaybuffered.MakeBuilder().WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 1, OutgoingCapacity: 1}).Build()
		sw.BindPort(fmt.Sprintf("Links[%d]", i), ports[i])
		(&tracingTestConnection{}).BindPort(ports[i])
	}
	require.NoError(t, s.Initialize())

	return sw, ports, s.Engine().(*timing.SerialEngine)
}

func TestSwitchSleepsWhileFlitsTravel(t *testing.T) {
	const latency = 100
	sw, ports, engine := sleepSwitch(t, latency)
	counter := &tickCounter{name: sw.Name()}
	engine.AcceptHook(counter)

	ports[0].Deliver(messaging.Msg{ID: 1, Payload: packetization.Flit{Dst: "Destination"}})
	require.NoError(t, engine.Run())

	msg, ok := ports[1].RetrieveOutgoing()
	require.True(t, ok, "the flit must leave the switch")
	require.Equal(t, uint64(1), msg.ID)
	require.GreaterOrEqual(t, engine.CurrentTime(), latency*sw.Spec.Freq.Period())
	require.Less(t, counter.ticks, 12, "the switch must not tick while a flit only travels")
}

type sendCycleRecorder struct {
	engine *timing.SerialEngine
	freq   timing.Freq
	cycles []uint64
}

func (r *sendCycleRecorder) Func(ctx hooking.HookCtx) {
	if ctx.Pos == messaging.HookPosPortMsgSend {
		r.cycles = append(r.cycles, r.freq.Cycle(r.engine.CurrentTime()))
	}
}

// TestFirstTickAtEdgeZeroMatchesForcedTicks starts the switch with TickAt(0)
// while a flit is already waiting at its input.
func TestFirstTickAtEdgeZeroMatchesForcedTicks(t *testing.T) {
	for _, latency := range []int{1, 2, 5} {
		run := func(forceTicks bool) []uint64 {
			sw, ports, engine := sleepSwitch(t, latency)
			rec := &sendCycleRecorder{engine: engine, freq: sw.Spec.Freq}
			ports[1].AcceptHook(rec)
			ports[0].Deliver(messaging.Msg{ID: 1, Payload: packetization.Flit{Dst: "Destination"}})
			sw.TickAt(0)
			if forceTicks {
				for cycle := uint64(1); cycle < 30; cycle++ {
					sw.TickAt(timing.VTimeInPicoSec(cycle) * sw.Spec.Freq.Period())
				}
			}
			require.NoError(t, engine.Run())
			return rec.cycles
		}

		sparse, forced := run(false), run(true)

		require.Len(t, sparse, 1, "latency %d", latency)
		require.Equal(t, forced, sparse, "latency %d", latency)
	}
}

func TestSwitchWakesBlockedPipelineAfterBackpressure(t *testing.T) {
	const latency = 5
	sw, ports, engine := sleepSwitch(t, latency)

	const flits = 40
	next, received := uint64(1), 0
	seen := map[uint64]bool{}
	for received < flits {
		for next <= flits && ports[0].CanDeliver() {
			ports[0].Deliver(messaging.Msg{ID: next, Payload: packetization.Flit{Dst: "Destination"}})
			next++
		}
		require.NoError(t, engine.RunUntil(engine.CurrentTime()+200*sw.Spec.Freq.Period()))

		msg, ok := ports[1].RetrieveOutgoing()
		require.True(t, ok, "a blocked switch must resume after each retrieval")
		require.False(t, seen[msg.ID], "duplicate flit")
		seen[msg.ID] = true
		received++
	}
}
