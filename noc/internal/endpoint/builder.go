package endpoint

import (
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Builder creates an endpoint and its device-side connection together.
type Builder struct {
	builder ticking.Builder[Spec, state, Resources, Ports, middlewares]
}

// MakeBuilder starts with the endpoint's default configuration.
func MakeBuilder() Builder { return Builder{builder: Definition.Builder()} }

// WithSimulation sets the owning simulation.
func (b Builder) WithSimulation(s timing.Simulation) Builder {
	b.builder = b.builder.WithSimulation(s)
	return b
}

// WithSpec sets the endpoint and device routing configuration.
func (b Builder) WithSpec(spec Spec) Builder { b.builder = b.builder.WithSpec(spec); return b }

// Build creates a fresh connection for each endpoint, ready for device binding.
func (b Builder) Build(name string) *Comp {
	devices := &deviceSide{}
	c := b.builder.WithResources(Resources{devices: devices}).Build(name)
	devices.Comp = c
	return c
}
