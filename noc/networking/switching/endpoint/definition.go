package endpoint

import (
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the endpoint, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:              1 * timing.GHz,
		NumInputChannels:  1,
		NumOutputChannels: 1,
		FlitByteSize:      32,
		EncodingOverhead:  0.25,
	},
	NewMiddlewares: newMiddlewares,
}

// newMiddlewares creates behavior over the device ports wired by ConnectDevices.
func newMiddlewares(c *Comp) middlewares {
	devicePorts := c.Resources.DevicePorts

	return middlewares{
		Outgoing: &outgoingMW{comp: c, devicePorts: devicePorts},
		Incoming: &incomingMW{comp: c, devicePorts: devicePorts},
	}
}
