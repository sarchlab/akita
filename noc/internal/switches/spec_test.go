package switches_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

func TestLinkConfigurationAndBindingValidation(t *testing.T) {
	s := sim.MakeBuilder().Build()
	spec := switches.DefaultSpec()
	spec.Links = []switches.Link{{Remote: "Peer.Top", Latency: 2, NumInputChannel: 1, NumOutputChannel: 1}}
	sw := modeling.MakeBuilder[*switches.Comp](spec).WithSimulation(s).Build("Switch")
	// Build owns its copy and permits ports to be bound later.
	spec.Links[0].Latency = 99
	require.Equal(t, 2, sw.Spec.Links[0].Latency)
	require.Empty(t, sw.State.PortComplexes)
	require.PanicsWithError(t, "Switch: switches: 0 ports but 1 links in Spec.Links", func() { _ = s.Initialize() })
	require.Error(t, s.Initialize())
	require.Error(t, s.Run())
	require.Empty(t, sw.State.PortComplexes)
	s = sim.MakeBuilder().Build()
	spec.Links[0].Latency = 2
	sw = modeling.MakeBuilder[*switches.Comp](spec).WithSimulation(s).Build("Switch")
	sw.Ports.Links = make([]messaging.Port, 1)
	port := twowaybuffered.NewPort(1, 1)
	sw.BindPort("Links[0]", port)
	require.NoError(t, s.Initialize())
	require.Len(t, sw.State.PortComplexes, 1)
	require.Equal(t, 2, sw.State.PortComplexes[0].Latency)
}

func TestInvalidLinkRejectedAtInitialization(t *testing.T) {
	s := sim.MakeBuilder().Build()
	spec := switches.DefaultSpec()
	spec.Links = []switches.Link{{Remote: "Peer.Top", Latency: -1, NumInputChannel: 1, NumOutputChannel: 1}}
	sw := modeling.MakeBuilder[*switches.Comp](spec).WithSimulation(s).Build("Switch")
	sw.BindPort("Links[0]", twowaybuffered.NewPort(1, 1))
	require.Panics(t, func() { _ = s.Initialize() })
	require.Empty(t, sw.State.PortComplexes)
}
