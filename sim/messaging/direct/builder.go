package direct

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition supplies the default configuration and builder for direct connections.
var Definition = definition{DefaultSpec: Spec{Freq: 1 * timing.GHz}}

type definition struct {
	DefaultSpec Spec
}

// Builder creates a builder initialized with the definition's default configuration.
func (d definition) Builder() Builder {
	return Builder{spec: d.DefaultSpec}
}

// Builder builds direct connections. A connection owns no ports (ports plug in)
// and has no resources, so it is configured by Spec alone and wired to the
// simulation through WithSimulation.
type Builder struct {
	spec       Spec
	simulation timing.Simulation
}

// WithSimulation sets the simulation that owns and registers the built connection.
func (b Builder) WithSimulation(sim timing.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec sets the entire configuration. Start from Definition.DefaultSpec and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

func (b Builder) Build(name string) *Connection {
	if b.simulation == nil {
		panic("direct: WithSimulation is required")
	}

	naming.MustBeValid(name)
	modeling.MustBeCheckpointable[Spec, State](name, b.spec)

	conn := &Connection{
		name: name,
		spec: b.spec,
		// A direct connection ticks on secondary events, so it runs after the
		// components of the same cycle.
		ticks: ticking.NewSecondaryScheduler(name, b.simulation, b.spec.Freq),
		ports: ports{portMap: make(map[messaging.RemotePort]int)},
	}

	b.simulation.Engine().RegisterHandler(name, conn)

	b.simulation.RegisterConnection(conn)

	return conn
}
