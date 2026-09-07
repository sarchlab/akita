package endpoint

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for endpoint components.
var defaultSpec = Spec{
	Freq:              1 * timing.GHz,
	NumInputChannels:  1,
	NumOutputChannels: 1,
	FlitByteSize:      32,
	EncodingOverhead:  0.25,
}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds End Points. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithSimulation and WithResources. The
// component declares a "NetworkPort"; the instance is assigned externally after
// Build (e.g. with SetNetworkPort / AssignPort).
type Builder struct {
	simulation timing.Simulation
	spec       Spec
	resources  Resources
}

// MakeBuilder creates a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: defaultSpec,
	}
}

// WithSimulation sets the simulation that owns and registers the built component.
func (b Builder) WithSimulation(sim timing.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the external wiring (device ports plugged into the
// endpoint). If not set, the endpoint is built without device ports and they
// can be plugged in later with PlugIn.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build creates a new End Point. It declares the component's "NetworkPort"; the
// instance is assigned externally after Build (see SetNetworkPort).
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("endpoint: WithSimulation is required")
	}

	spec := b.spec
	sim := b.simulation

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithSimulation(sim).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	ep := &Comp{
		Component: modelComp,
	}

	outMW := &outgoingMW{
		comp:             modelComp,
		defaultSwitchDst: spec.DefaultSwitchDst,
	}

	inMW := &incomingMW{
		comp: modelComp,
	}

	ep.AddMiddleware(outMW)
	ep.AddMiddleware(inMW)

	ep.DeclarePort("NetworkPort", packetization.Link)

	for _, dp := range b.resources.DevicePorts {
		ep.PlugIn(dp)
	}

	b.simulation.RegisterComponent(ep)

	return ep
}
