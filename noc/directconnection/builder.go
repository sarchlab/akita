package directconnection

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	// Builder can help building directconnection.
	"github.com/sarchlab/akita/v5/messaging"
)

// defaultSpec provides the default configuration for a direct connection.
var defaultSpec = Spec{Freq: 1 * timing.GHz}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds direct connections. A connection owns no ports (ports plug in)
// and has no resources, so it is configured by Spec alone and wired to the
// simulation through WithSimulation.
type Builder struct {
	spec       Spec
	simulation timing.Simulation
}

func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithSimulation sets the simulation that owns and registers the built connection.
func (b Builder) WithSimulation(sim timing.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("directconnection: WithSimulation is required")
	}

	sim := b.simulation
	spec := b.spec

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithSimulation(sim).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	// DirectConnection uses secondary tick events so it runs after primary
	// components. Replace the primary TickingComponent created by the builder
	// with a secondary one. Since SerialEngine.RegisterHandler overwrites by
	// name, the final registration is for the secondary component. ✓
	modelComp.TickingComponent = modeling.NewSecondaryTickingComponent(
		name, sim, spec.Freq, modelComp)

	mw := &middleware{
		comp: modelComp,
		ports: ports{
			ports:   make([]messaging.Port, 0),
			portMap: make(map[messaging.RemotePort]int),
		},
	}
	modelComp.AddMiddleware(mw)

	conn := &Comp{Component: modelComp}

	b.simulation.RegisterConnection(conn)

	return conn
}
