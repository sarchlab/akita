package twowaybuffered_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestBuilderRequiresSimulation(t *testing.T) {
	require.PanicsWithValue(t, "twowaybuffered: WithSimulation is required", func() { MakeBuilder().Build() })
	var absent *sim.Simulation
	require.PanicsWithValue(t, "twowaybuffered: WithSimulation is required", func() {
		MakeBuilder().
			WithSimulation(absent).
			Build()
	})
}

func TestBuilderDefaultsAndIndependentPorts(t *testing.T) {
	s := sim.MakeBuilder().Build()
	builder := MakeBuilder().WithSimulation(s)
	spec := DefaultSpec()
	require.Equal(t, Spec{IncomingCapacity: 1, OutgoingCapacity: 1}, spec)
	spec.IncomingCapacity = 3
	spec.OutgoingCapacity = 2
	custom := builder.WithSpec(spec).Build()
	first, second := builder.Build(), builder.Build()
	require.Empty(t, s.Ports(), "Build must leave the port unregistered")
	require.Empty(t, first.Name())
	require.Nil(t, first.Owner())
	require.Nil(t, first.Connection())
	require.Equal(t, 1, first.IncomingBuf().Capacity())
	require.Equal(t, 1, first.OutgoingBuf().Capacity())
	require.Equal(t, 3, custom.IncomingBuf().Capacity())
	require.Equal(t, 2, custom.OutgoingBuf().Capacity())
	first.BindOwner(&portPresenceOwner{}, "First")
	first.Deliver(messaging.Msg{ID: 1})
	require.False(t, first.CanDeliver())
	require.True(t, second.CanDeliver(), "Build calls must not share buffer state")
	require.NoError(t, s.Initialize())
	require.Panics(t, func() { builder.Build() })
	t.Log("Default ports have independent 1/1 buffers; custom capacities are 3/2; Build leaves inventory empty")
}
