package ticking

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/internal/tickscheduler"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/internal/base"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var freqType = reflect.TypeFor[timing.Freq]()

// Builder builds instances of one ticking component type. Obtain it with
// modeling.MakeBuilder[*Comp](DefaultSpec()).
type Builder[S, T, R, P, M any] struct {
	simulation modeling.Simulation
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
	sim modeling.Simulation,
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
		panic(fmt.Sprintf("ticking: %s %q: WithSimulation is required",
			typeName[S](), name))
	}

	assembly.SetupOf(b.simulation).RequireNameAvailable(name)
	modeling.MustBeCheckpointable[S, T](name, b.spec)
	c := &Component[S, T, R, P, M]{
		ticks: tickscheduler.NewScheduler(name, b.simulation, specFreq(name, b.spec)),
	}
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

// specFreq returns the Spec's Freq field, the clock the instance ticks at.
func specFreq(name string, spec any) timing.Freq {
	v := reflect.ValueOf(spec)

	f := v.FieldByName("Freq")
	if !f.IsValid() || f.Type() != freqType {
		panic(fmt.Sprintf(
			"ticking: component %q: Spec %s must have a Freq timing.Freq field", name, v.Type()))
	}

	freq := f.Interface().(timing.Freq)
	if freq == 0 {
		panic(fmt.Sprintf("ticking: component %q: Spec %s has zero Freq", name, v.Type()))
	}
	return freq
}

// typeName returns the import path of the package that declares S, which
// identifies the component type.
func typeName[S any]() string {
	return reflect.TypeFor[S]().PkgPath()
}
