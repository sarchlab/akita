package endpoint

import (
	"fmt"
	"testing"

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
	for _, unsupported := range []messaging.Port{
		&unsupportedDevicePort{}, nil, (*twowaybuffered.Port)(nil),
	} {
		t.Run(fmt.Sprintf("%T", unsupported), func(t *testing.T) {
			device := twowaybuffered.NewPort(1, 1)
			device.BindOwner(payloadOwner{"Device"}, "Device.Port")
			s := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
			require.PanicsWithValue(t,
				fmt.Sprintf("endpoint: device port must be *twowaybuffered.Port, got %T", unsupported),
				func() {
					builtComponent := Definition.Builder().WithSimulation(s).
						WithResources(Resources{DevicePorts: []messaging.Port{device, unsupported}}).
						Build("EP")

					builtComponent.BindPort("NetworkPort", twowaybuffered.NewPort(1, 1))
					ConnectDevices(builtComponent)
					if err := s.Initialize(); err != nil {
						panic(err)
					}
				})
			require.Nil(t, device.Connection(), "valid port was attached before incompatible port was rejected")
		})
	}
	t.Log("Unsupported device ports were rejected during endpoint construction before any device attachment")
}

func TestRejectFrozenDeviceOwnerBeforeAttaching(t *testing.T) {
	frozen := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
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

	early := twowaybuffered.NewPort(1, 1)
	early.BindOwner(payloadOwner{"Device"}, "Device.Port")
	s := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
	ep := Definition.Builder().WithSimulation(s).
		WithResources(Resources{DevicePorts: []messaging.Port{early, late}}).Build("EP")
	require.Panics(t, func() { ConnectDevices(ep) })
	require.Nil(t, early.Connection(), "earlier devices must stay untouched when a later owner rejects attachment")
	require.Nil(t, late.Connection())
}
