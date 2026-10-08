package endpoint

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
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
			device := twowaybuffered.NewPort("Device.Port", 1, 1)
			s := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
			require.PanicsWithValue(t,
				fmt.Sprintf("endpoint: device port must be *twowaybuffered.Port, got %T", unsupported),
				func() {
					Definition.Builder().WithSimulation(s).
						WithResources(Resources{DevicePorts: []messaging.Port{device, unsupported}}).
						WithPorts(Ports{NetworkPort: twowaybuffered.NewPort("EP.NetworkPort", 1, 1)}).
						Build("EP")
				})
			require.Nil(t, device.Connection(), "valid port was attached before incompatible port was rejected")
		})
	}
	t.Log("Unsupported device ports were rejected during endpoint construction before any device attachment")
}
