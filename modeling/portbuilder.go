package modeling

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// PortSpec configures a port built by a PortBuilder.
type PortSpec struct {
	// BufSize is the capacity of both the incoming and outgoing buffers.
	BufSize int
}

// defaultPortSpec is the default port configuration.
var defaultPortSpec = PortSpec{BufSize: 1}

// DefaultPortSpec returns a copy of the default port configuration. Callers
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultPortSpec() PortSpec {
	return defaultPortSpec
}

// PortBuilder builds a messaging.Port and registers it with the simulation,
// mirroring how component and connection builders register themselves. The
// component owns its port topology; a PortBuilder supplies an instance for one
// of those ports, choosing its buffer size.
type PortBuilder struct {
	simulation timing.Simulation
	comp       messaging.Component
	spec       PortSpec
}

// MakePortBuilder returns a PortBuilder seeded with the default spec.
func MakePortBuilder() PortBuilder {
	return PortBuilder{spec: defaultPortSpec}
}

// WithSimulation sets the simulation that registers the built port.
func (b PortBuilder) WithSimulation(sim timing.Simulation) PortBuilder {
	b.simulation = sim
	return b
}

// WithComponent sets the component that owns the port.
func (b PortBuilder) WithComponent(comp messaging.Component) PortBuilder {
	b.comp = comp
	return b
}

// WithSpec sets the port configuration. Start from DefaultPortSpec() and tweak.
func (b PortBuilder) WithSpec(spec PortSpec) PortBuilder {
	b.spec = spec
	return b
}

// Build builds a port and registers it with the simulation.
//
// With a component, the port's full name is comp.Name()+"."+name and the
// component owns it; attach it with comp.AssignPort(name, port). Without one,
// name is the full name, "<component>.<port>", and the port is unowned until
// a Comp builder binds it (see CompBuilder.WithPorts).
func (b PortBuilder) Build(name string) messaging.Port {
	if b.simulation == nil {
		panic("modeling: PortBuilder requires a simulation")
	}

	fullName := name
	if b.comp != nil {
		fullName = b.comp.Name() + "." + name
	}

	port := messaging.NewPort(b.comp, b.spec.BufSize, b.spec.BufSize, fullName)
	b.simulation.RegisterPort(port)

	return port
}
