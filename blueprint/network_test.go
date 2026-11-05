package blueprint_test

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestCustomConnectorIsDeferred(t *testing.T) {
	called := false
	var bp blueprint.Blueprint
	bp.Add("A", comp(testSpec{}))
	bp.Add("B", comp(testSpec{}))
	n := blueprint.NewNetwork(struct{ Freq timing.Freq }{timing.GHz}, []messaging.RemotePort{bp.PortRef("A.Top"),
		bp.PortRef("B.Top")}, func(settings struct{ Freq timing.Freq }, s modeling.Simulation, name string,
		ports []messaging.Port) {
		called = true
		require.Equal(t, "A.Top", ports[0].Name())
		require.Equal(t, "B.Top", ports[1].Name())
		c := direct.NewConnection(name, s, settings.Freq)
		for _, p := range ports {
			c.BindPort(p)
		}
	})
	bp.Add("Custom", n)
	flat, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	require.False(t, called)
	require.IsType(t, &blueprint.NetworkNode{}, flat.Nodes()[2])
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	for _, n := range bp.Nodes() {
		s.Add(n.Name(), n)
	}
	require.NoError(t, s.Initialize())
	require.True(t, called)
}

func TestInvalidCustomNetworkSettings(t *testing.T) {
	var bp blueprint.Blueprint
	bp.Add("Network", blueprint.NewNetwork(invalidNetworkSpec{}, nil,
		func(invalidNetworkSpec, modeling.Simulation, string, []messaging.Port) {
			t.Fatal("must not instantiate")
		}))
	_, err := blueprint.Expand(&bp)
	require.ErrorContains(t, err, "Network.Spec: invalid network")
}

type invalidNetworkSpec struct{}

func (invalidNetworkSpec) Validate() error { return fmt.Errorf("invalid network") }
