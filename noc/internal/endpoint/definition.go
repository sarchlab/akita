package endpoint

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:              1 * timing.GHz,
		NumInputChannels:  1,
		NumOutputChannels: 1,
		FlitByteSize:      32,
		EncodingOverhead:  0.25,
	}
}

// Initialize creates behavior over the device ports bound to DeviceConnection.
func (m *middlewares) Initialize(c *Comp) {
	if c.Resources.devices == nil {
		panic("endpoint: construct endpoints with MakeBuilder")
	}
	peer := (&topology{comp: c}).record().Peer
	devicePorts := c.Resources.devices.ports

	*m = middlewares{
		Outgoing: &outgoingMW{comp: c, devicePorts: devicePorts, peer: peer},
		Incoming: &incomingMW{comp: c, devicePorts: devicePorts},
	}
}
