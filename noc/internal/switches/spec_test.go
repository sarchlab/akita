package switches_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/noc/internal/switches"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/stretchr/testify/require"
)

func TestLinkConfigurationAndBindingValidation(t *testing.T) {
	s := sim.MakeBuilder().Build()
	spec := switches.Definition.DefaultSpec
	spec.Links = []switches.Link{{Remote: "Peer.Top", Latency: 2, NumInputChannel: 1, NumOutputChannel: 1}}
	sw := switches.Definition.Builder().WithSimulation(s).WithSpec(spec).Build("Switch")
	// Build owns its copy and permits ports to be bound later.
	spec.Links[0].Latency = 99
	require.Equal(t, 2, sw.Spec.Links[0].Latency)
	require.Empty(t, sw.State.PortComplexes)
	require.ErrorContains(t, s.Initialize(), "0 ports but 1 links")
	require.Empty(t, sw.State.PortComplexes)
	sw.Ports.Links = make([]messaging.Port, 1)
	port := twowaybuffered.NewPort(1, 1)
	sw.BindPort("Links[0]", port)
	require.NoError(t, s.Initialize())
	require.Len(t, sw.State.PortComplexes, 1)
	require.Equal(t, 2, sw.State.PortComplexes[0].Latency)
}

func TestInvalidLinkRejectedAtBuild(t *testing.T) {
	s := sim.MakeBuilder().Build()
	spec := switches.Definition.DefaultSpec
	spec.Links = []switches.Link{{Remote: "Peer.Top", Latency: -1, NumInputChannel: 1, NumOutputChannel: 1}}
	require.Panics(t, func() {
		switches.Definition.Builder().WithSimulation(s).WithSpec(spec).Build("Switch")
	})
}
