package ticking

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/modeling"
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

// WithPorts sets the instance's ports. Every fixed port must be given and
// named "<instance>.<field>"; port groups may start empty and grow with
// AssignPortToGroup.
func (b Builder[S, T, R, P, M]) WithPorts(ports P) Builder[S, T, R, P, M] {
	b.ports = ports
	return b
}

// Build creates the instance with the given name. It checks the
// configuration, binds the ports, creates the State with NewState and the
// middlewares with NewMiddlewares, and registers the instance with the
// simulation, in that order.
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
		Ports:     b.ports,
		name:      name,
		spec:      b.spec,
		resources: b.resources,
	}
	c.TickScheduler = modeling.NewTickScheduler(name, b.simulation, specFreq(b.spec))
	c.ports = modeling.NewPortTable(c, &c.Ports)

	if b.def.NewState != nil {
		c.State = b.def.NewState(name, b.spec)
	}

	c.Middlewares = b.def.NewMiddlewares(c)
	c.pipeline = modeling.OrderedMiddlewares(&c.Middlewares)

	if handlers, ok := b.simulation.GetEngine().(timing.HandlerRegistry); ok {
		handlers.RegisterHandler(name, c)
	}

	b.simulation.RegisterComponent(c)

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
