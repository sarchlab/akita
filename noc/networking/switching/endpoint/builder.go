package endpoint

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for endpoint components.
var defaultSpec = Spec{
	Freq:                  1 * timing.GHz,
	NumInputChannels:      1,
	NumOutputChannels:     1,
	FlitByteSize:          32,
	EncodingOverhead:      0.25,
	NetworkPortBufferSize: 4,
}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds End Points. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithRegistrar and WithResources. The
// component creates its own network port.
type Builder struct {
	registrar modeling.Registrar
	spec      Spec
	resources Resources
}

// MakeBuilder creates a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: defaultSpec,
	}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
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

// Build creates a new End Point. It creates the component's network port.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("endpoint: WithRegistrar is required")
	}

	spec := b.spec
	engine := b.registrar.GetEngine()

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	ep := &Comp{
		Component: modelComp,
	}

	networkPort := messaging.NewPort(
		ep, spec.NetworkPortBufferSize, spec.NetworkPortBufferSize,
		name+".NetworkPort")

	outMW := &outgoingMW{
		comp:             modelComp,
		networkPort:      networkPort,
		defaultSwitchDst: spec.DefaultSwitchDst,
	}

	inMW := &incomingMW{
		comp:        modelComp,
		networkPort: networkPort,
	}

	ep.AddMiddleware(outMW)
	ep.AddMiddleware(inMW)

	for _, dp := range b.resources.DevicePorts {
		ep.PlugIn(dp)
	}

	b.registrar.RegisterComponent(ep)

	return ep
}
