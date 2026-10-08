package ticking

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/internal/base"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var freqType = reflect.TypeFor[timing.Freq]()

// Builder builds instances of one ticking component type. Obtain it with
// Definition.Builder.
type Builder[S, T, R, P, M any] struct {
	def        Definition[S, T, R, P, M]
	simulation timing.Simulation
	spec       S
	resources  R
}

// WithSimulation sets the simulation the instance belongs to and registers
// with.
func (b Builder[S, T, R, P, M]) WithSimulation(
	sim timing.Simulation,
) Builder[S, T, R, P, M] {
	b.simulation = sim
	return b
}

// WithSpec sets the configuration. Start from the definition's DefaultSpec.
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
		panic(fmt.Sprintf("ticking: %s %q: WithSimulation is required",
			typeName[S](), name))
	}

	if b.def.NewMiddlewares == nil {
		panic(fmt.Sprintf("ticking: %s: Definition has no NewMiddlewares",
			typeName[S]()))
	}

	b.simulation.RequireNameAvailable(name)
	modeling.MustBeCheckpointable[S, T](name, b.spec)

	c := &Component[S, T, R, P, M]{
		ticks: NewScheduler(name, b.simulation, specFreq(b.spec)),
	}
	base.Init(&c.ComponentBase, c,
		b.simulation, name, b.spec, b.resources)

	base.SetInitializer(&c.ComponentBase, func() {
		if b.def.NewState != nil {
			c.State = b.def.NewState(c)
		}

		c.Middlewares = b.def.NewMiddlewares(c)
		c.pipeline = base.OrderedMiddlewares(&c.Middlewares)
	})

	base.Register(&c.ComponentBase)

	return c
}

// specFreq returns the Spec's Freq field, the clock the instance ticks at.
func specFreq(spec any) timing.Freq {
	v := reflect.ValueOf(spec)

	f := v.FieldByName("Freq")
	if !f.IsValid() || f.Type() != freqType {
		panic(fmt.Sprintf(
			"ticking: Spec %s must have a Freq timing.Freq field", v.Type()))
	}

	return f.Interface().(timing.Freq)
}

// typeName returns the import path of the package that declares S, which
// identifies the component type.
func typeName[S any]() string {
	return reflect.TypeFor[S]().PkgPath()
}
