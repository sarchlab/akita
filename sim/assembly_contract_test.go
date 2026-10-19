package sim_test

import (
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type assemblyPorts struct{ In messaging.Port }
type assemblyComponent = event.Component[modeling.None, modeling.None, modeling.None, assemblyPorts, modeling.None]

func TestCrossSimulationBindingIsAtomic(t *testing.T) {
	for _, tracking := range []bool{false, true} {
		builder := sim.MakeBuilder()
		if tracking {
			builder = builder.WithTaskTracking()
		}
		left, right := builder.Build(), builder.Build()
		comp := modeling.MakeBuilder[*assemblyComponent](modeling.None{}).WithSimulation(left).Build("Agent")
		foreign := twowaybuffered.MakeBuilder().WithSimulation(right).Build()
		require.Panics(t, func() { comp.BindPort("In", foreign) })
		require.Nil(t, comp.Ports.In)
		require.Nil(t, foreign.Owner())
		require.Empty(t, foreign.Name())
		require.Empty(t, left.Ports())
		own := twowaybuffered.MakeBuilder().WithSimulation(left).Build()
		comp.BindPort("In", own)
		require.Equal(t, []messaging.Port{own}, left.Ports())
		require.Same(t, comp, own.Owner())
		require.Equal(t, "Agent.In", own.Name())
		wrongConn := direct.NewConnection("OtherLink", right, timing.GHz)
		require.Panics(t, func() { wrongConn.BindPort(own) })
		require.Nil(t, own.Connection())
		link := direct.NewConnection("Link", left, timing.GHz)
		link.BindPort(own)
		require.Same(t, link, own.Connection())
		late := twowaybuffered.MakeBuilder().WithSimulation(left).Build()
		require.NoError(t, left.Initialize())
		require.Panics(t, func() { comp.BindPort("In", late) })
		require.Nil(t, late.Owner())
		require.Empty(t, late.Name())
		require.Panics(t, func() { late.BindOwner(comp, "Agent.Other") })
	}
	t.Log("Component and connection reject foreign ports without mutation, tracking on or off; " +
		"local binding registers both sides and freezes at Initialize")
}

func TestFrameworkLifecycleIsNotPublic(t *testing.T) {
	for _, value := range []any{
		(*sim.Simulation)(nil), (*assemblyComponent)(nil),
		(*ticking.Component[modeling.None, modeling.None, modeling.None, modeling.None, modeling.None])(nil),
		(*wakeup.Component[modeling.None, modeling.None, modeling.None, modeling.None, modeling.None])(nil),
		(*twowaybuffered.Port)(nil), (*direct.Connection)(nil),
	} {
		for _, method := range []string{
			"RequireSetup", "RequireNameAvailable", "ValidateSetup",
			"InitializeComponent", "RequireInitialized", "BindTaskTracker",
		} {
			_, public := reflect.TypeOf(value).MethodByName(method)
			require.False(t, public, "%T exposes framework method %s", value, method)
		}
	}
}
