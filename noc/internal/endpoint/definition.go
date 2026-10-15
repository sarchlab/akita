package endpoint

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the endpoint, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// MakeBuilder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:              1 * timing.GHz,
		NumInputChannels:  1,
		NumOutputChannels: 1,
		FlitByteSize:      32,
		EncodingOverhead:  0.25,
	},
	ValidateSpec:   validateSpec,
	ValidateSetup:  validateSetup,
	NewMiddlewares: newMiddlewares,
}

// newMiddlewares creates behavior over the device ports bound to DeviceConnection.
func newMiddlewares(c *Comp) middlewares {
	devicePorts := c.Resources.devices.ports

	return middlewares{
		Outgoing: &outgoingMW{comp: c, devicePorts: devicePorts},
		Incoming: &incomingMW{comp: c, devicePorts: devicePorts},
	}
}

func validateSpec(spec Spec) error {
	seen := map[string]bool{}
	for _, device := range spec.Devices {
		if device == "" || seen[string(device)] {
			return fmt.Errorf("endpoint: Devices must contain distinct nonempty addresses")
		}
		seen[string(device)] = true
	}
	return nil
}

func validateSetup(c *Comp) error {
	if c.Resources.devices == nil {
		return fmt.Errorf("endpoint: construct endpoints with MakeBuilder")
	}
	ports := c.Resources.devices.ports
	if len(ports) != len(c.Spec.Devices) {
		return fmt.Errorf("endpoint: bound device count %d does not match Spec.Devices length %d",
			len(ports), len(c.Spec.Devices))
	}
	for i, port := range ports {
		if port.AsRemote() != c.Spec.Devices[i] || port.Connection() != c.Resources.devices {
			return fmt.Errorf("endpoint: bound device %d does not match Spec.Devices or its connection", i)
		}
	}
	return nil
}
