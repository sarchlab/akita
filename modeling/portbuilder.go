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

// Build builds a port named comp.Name()+"."+name, owned by the component, and
// registers it with the simulation. Attach it with comp.AssignPort(name, port).
//
// PortBuilder serves components on the Component API. A component defined by
// a component model (modeling/ticking and its siblings) takes ports created
// with messaging.NewPort through its builder's WithPorts, and its Build
// registers them.
func (b PortBuilder) Build(name string) messaging.Port {
	if b.simulation == nil {
		panic("modeling: PortBuilder requires a simulation")
	}

	if b.comp == nil {
		panic("modeling: PortBuilder requires a component; for a component " +
			"model, create the port with messaging.NewPort and pass it to WithPorts")
	}

	port := messaging.NewPort(b.comp, b.spec.BufSize, b.spec.BufSize,
		b.comp.Name()+"."+name)
	b.simulation.RegisterPort(port)

	return port
}
