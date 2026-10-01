package ticking

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/internal/base"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

var freqType = reflect.TypeFor[timing.Freq]()

// Builder builds instances of one ticking component type. Obtain it with
// Definition.Builder.
type Builder[S, T, R, P, M any] struct {
	def        Definition[S, T, R, P, M]
	simulation timing.Simulation
	spec       S
	resources  R
	ports      P
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

// WithPorts sets the instance's ports, created by the system builder with
// messaging.NewPort. Every port must be given and named "<instance>.<field>",
// or "<instance>.<field>[i]" for member i of a port group.
func (b Builder[S, T, R, P, M]) WithPorts(ports P) Builder[S, T, R, P, M] {
	b.ports = ports
	return b
}

// Build creates the instance with the given name. It checks the
// configuration, binds the ports, creates the State with NewState and the
// middlewares with NewMiddlewares, and registers the ports and the instance
// with the simulation, in that order, so a Build that fails registers nothing.
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

	modeling.MustBeCheckpointable[S, T](name, b.spec)

	c := &Component[S, T, R, P, M]{
		ticks: NewScheduler(name, b.simulation, specFreq(b.spec)),
	}
	base.Init(&c.ComponentBase, c,
		b.simulation, name, b.spec, b.resources, b.ports)

	if b.def.NewState != nil {
		c.State = b.def.NewState(c)
	}

	c.Middlewares = b.def.NewMiddlewares(c)
	c.pipeline = base.OrderedMiddlewares(&c.Middlewares)

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
