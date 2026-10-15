package endpoint

import (
	"fmt"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// Merely implementing Port does not make a different transport compatible.
// Embedding it lets the test detect rejection without invoking any operation.
type unsupportedDevicePort struct{ messaging.Port }

func TestRejectUnsupportedDevicePortsBeforeAttaching(t *testing.T) {
	s := akitasim.MakeBuilder().Build()
	ep := MakeBuilder().WithSimulation(s).Build("EP")
	for _, unsupported := range []messaging.Port{&unsupportedDevicePort{}, nil, (*twowaybuffered.Port)(nil)} {
		require.PanicsWithValue(t,
			fmt.Sprintf("endpoint: device port must be *twowaybuffered.Port, got %T", unsupported),
			func() { DeviceConnection(ep).BindPort(unsupported) })
		require.Empty(t, ep.Resources.devices.ports)
	}
}

func TestRejectFrozenDeviceOwnerBeforeAttaching(t *testing.T) {
	frozen := akitasim.MakeBuilder().Build()
	def := ticking.Definition[Spec, modeling.None, modeling.None, Ports, modeling.None]{
		DefaultSpec: Spec{Freq: timing.GHz},
		NewMiddlewares: func(*ticking.Component[Spec, modeling.None, modeling.None, Ports, modeling.None]) modeling.None {
			return modeling.None{}
		},
	}
	owner := def.Builder().WithSimulation(frozen).Build("FrozenDevice")
	late := twowaybuffered.NewPort(1, 1)
	owner.BindPort("NetworkPort", late)
	require.NoError(t, frozen.Initialize())

	s := akitasim.MakeBuilder().Build()
	ep := MakeBuilder().WithSimulation(s).Build("EP")
	require.Panics(t, func() { DeviceConnection(ep).BindPort(late) })
	require.Empty(t, ep.Resources.devices.ports)
	require.Nil(t, late.Connection())
}
