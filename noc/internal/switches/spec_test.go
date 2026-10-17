package switches_test

import (
	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
	"testing"
)

type fixtureWiring struct {
	ports []messaging.Port
	links []switches.Link
}

func (w *fixtureWiring) Ports() []messaging.Port       { return w.ports }
func (w *fixtureWiring) Links() []switches.Link        { return w.links }
func (w *fixtureWiring) Routes() switches.RoutingTable { return nil }

func TestLinkConfigurationAndBindingValidation(t *testing.T) {
	s := sim.MakeBuilder().Build()
	wiring := &fixtureWiring{links: []switches.Link{{Remote: "Peer.Top",
		Latency:          2,
		NumInputChannel:  1,
		NumOutputChannel: 1}}}
	sw := modeling.MakeBuilder[*switches.Comp](switches.DefaultSpec()).WithSimulation(s).
		WithResources(switches.Resources{Wiring: wiring}).Build("Switch")
	require.Empty(t, sw.State.PortComplexes)
	require.PanicsWithValue(t, "Switch: switches: 0 ports but 1 link settings", func() { _ = s.Initialize() })
	require.Error(t, s.Initialize())
	require.Error(t, s.Run())
	require.Empty(t, sw.State.PortComplexes)

	s = sim.MakeBuilder().Build()
	port := twowaybuffered.NewPort(1, 1)
	wiring.ports = []messaging.Port{port}
	sw = modeling.MakeBuilder[*switches.Comp](switches.DefaultSpec()).WithSimulation(s).
		WithResources(switches.Resources{Wiring: wiring}).Build("Switch")
	sw.BindPort("Links[0]", port)
	require.NoError(t, s.Initialize())
	require.Len(t, sw.State.PortComplexes, 1)
	require.Equal(t, 2, sw.State.PortComplexes[0].Latency)
}
func TestInvalidLinkRejectedAtInitialization(t *testing.T) {
	s := sim.MakeBuilder().Build()
	port := twowaybuffered.NewPort(1, 1)
	wiring := &fixtureWiring{ports: []messaging.Port{port},
		links: []switches.Link{{Remote: "Peer.Top",
			Latency:          -1,
			NumInputChannel:  1,
			NumOutputChannel: 1}}}
	sw := modeling.MakeBuilder[*switches.Comp](switches.DefaultSpec()).WithSimulation(s).
		WithResources(switches.Resources{Wiring: wiring}).Build("Switch")
	sw.BindPort("Links[0]", port)
	require.Panics(t, func() { _ = s.Initialize() })
	require.Empty(t, sw.State.PortComplexes)
}
