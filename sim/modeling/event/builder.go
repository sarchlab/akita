package event

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/internal/base"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Builder builds instances of one event component type. Obtain it with
// modeling.MakeBuilder[*Comp](DefaultSpec()).
type Builder[S, T, R, P, M any] struct {
	simulation timing.Simulation
	spec       S
	resources  R
}

// MakeBuilder implements modeling.MakeBuilder for this component type.
// The receiver is unused and may be nil; the returned builder has no runtime
// state from an existing instance.
func (*Component[S, T, R, P, M]) MakeBuilder(spec S) Builder[S, T, R, P, M] {
	return Builder[S, T, R, P, M]{spec: spec}
}

// WithSimulation sets the simulation the instance belongs to and registers
// with.
func (b Builder[S, T, R, P, M]) WithSimulation(
	sim timing.Simulation,
) Builder[S, T, R, P, M] {
	b.simulation = sim
	return b
}

// WithSpec sets the configuration. Start from DefaultSpec().
func (b Builder[S, T, R, P, M]) WithSpec(spec S) Builder[S, T, R, P, M] {
	b.spec = spec
	return b
}

// WithResources sets the references to shared objects.
func (b Builder[S, T, R, P, M]) WithResources(resources R) Builder[S, T, R, P, M] {
	b.resources = resources
	return b
}

// Build registers a configured instance with the given name. It checks Spec
// and State shapes, but leaves ports, State and Middlewares uninitialized.
// Bind the ports and connections, then call the simulation's Initialize.
func (b Builder[S, T, R, P, M]) Build(name string) *Component[S, T, R, P, M] {
	naming.MustBeValid(name)

	if b.simulation == nil {
		panic(fmt.Sprintf("event: %s %q: WithSimulation is required",
			typeName[S](), name))
	}

	b.simulation.RequireNameAvailable(name)
	modeling.MustBeCheckpointable[S, T](name, b.spec)
	c := &Component[S, T, R, P, M]{engine: b.simulation.Engine()}
	base.Init(&c.ComponentBase, c,
		b.simulation, name, b.spec, b.resources)

	base.SetInitializer(&c.ComponentBase, func() {
		base.InitializeElement(&c.State, c)
		base.InitializeElement(&c.Middlewares, c)
		c.pipeline = base.OrderedMiddlewares[Middleware](&c.Middlewares)
	})

	base.Register(&c.ComponentBase)

	return c
}

// typeName returns the import path of the package that declares S, which
// identifies the component type.
func typeName[S any]() string {
	return reflect.TypeFor[S]().PkgPath()
}
