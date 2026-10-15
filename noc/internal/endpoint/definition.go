package endpoint

import (
	"fmt"

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
func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}
	if err := validateSetup(c); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}

	devicePorts := c.Resources.devices.ports

	*initialMiddlewares = middlewares{
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
