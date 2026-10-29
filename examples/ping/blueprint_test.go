package ping

import (
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBlueprintBidirectionalPingDeclarationOrder(t *testing.T) {
	times := make([]timing.VTimeInPicoSec, 0, 2)
	for _, names := range [][]string{{"A", "B"}, {"B", "A"}} {
		s := sim.MakeBuilder().Build()
		s.Add("", blueprint.NewDomain[modeling.None, modeling.None](modeling.None{}, func(_ modeling.None,
			bp *blueprint.Blueprint) {
			for _, name := range names {
				bp.Add(name, blueprint.Comp[*Comp](DefaultSpec()).Port("Out",
					twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}))
			}
			bp.Connect("Wire", timing.GHz, bp.PortRef("A.Out"), bp.PortRef("B.Out"))
		}))
		require.NoError(t, s.Initialize())
		a := blueprint.Lookup[*Comp](s, "A")
		b := blueprint.Lookup[*Comp](s, "B")
		SchedulePing(a, 1, b.Ports.Out.AsRemote())
		SchedulePing(b, 2, a.Ports.Out.AsRemote())
		require.NoError(t, s.Run())
		require.Empty(t, a.State.ScheduledPings)
		require.Empty(t, b.State.ScheduledPings)
		require.Empty(t, a.State.PendingResponses)
		require.Empty(t, b.State.PendingResponses)
		times = append(times, s.Engine().CurrentTime())
		require.NoError(t, s.Terminate())
	}
	require.Equal(t, times[0], times[1])
}
