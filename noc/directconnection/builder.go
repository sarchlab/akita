package directconnection

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
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

	naming.MustBeValid(name)
	modeling.MustBeCheckpointable[Spec, State](name, b.spec)

	conn := &Comp{
		name: name,
		spec: b.spec,
		sim:  b.simulation,
		// A direct connection ticks on secondary events, so it runs after the
		// components of the same cycle.
		ticks: ticking.NewSecondaryScheduler(name, b.simulation, b.spec.Freq),
		ports: ports{portMap: make(map[messaging.RemotePort]int)},
	}

	if handlers, ok := b.simulation.Engine().(timing.HandlerRegistry); ok {
		handlers.RegisterHandler(name, conn)
	}

	b.simulation.RegisterConnection(conn)

	return conn
}
