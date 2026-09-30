package modeling

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// CompBuilder builds components of one Definition. Obtain it with
// Definition.Builder.
//
// CompBuilder is the prototype of the builder proposed in #492. It takes the
// name Builder once the legacy API is removed.
type CompBuilder[S, T, R, P, M any] struct {
	def        Definition[S, T, R, P, M]
	simulation timing.Simulation
	spec       S
	resources  R
	ports      P
}

// WithSimulation sets the simulation the component belongs to and registers
// with.
func (b CompBuilder[S, T, R, P, M]) WithSimulation(
	sim timing.Simulation,
) CompBuilder[S, T, R, P, M] {
	b.simulation = sim
	return b
}

// WithSpec sets the configuration. Start from the definition's DefaultSpec.
func (b CompBuilder[S, T, R, P, M]) WithSpec(spec S) CompBuilder[S, T, R, P, M] {
	b.spec = spec
	return b
}

// WithResources sets the references to shared objects.
func (b CompBuilder[S, T, R, P, M]) WithResources(
	resources R,
) CompBuilder[S, T, R, P, M] {
	b.resources = resources
	return b
}

// WithPorts sets the component's ports. Every fixed port must be given and
// named "<component>.<field>"; port groups may start empty and grow with
// AssignPortToGroup.
func (b CompBuilder[S, T, R, P, M]) WithPorts(ports P) CompBuilder[S, T, R, P, M] {
	b.ports = ports
	return b
}

// Build creates the component. It checks the configuration, binds the ports,
// creates the State with NewState and the middlewares with NewMiddlewares,
// and registers the component with the simulation, in that order.
func (b CompBuilder[S, T, R, P, M]) Build(name string) *Comp[S, T, R, P, M] {
	naming.MustBeValid(name)

	if b.simulation == nil {
		panic(fmt.Sprintf(
			"modeling: %s %q: WithSimulation is required", b.def.Name, name))
	}

	if b.def.NewMiddlewares == nil {
		panic(fmt.Sprintf(
			"modeling: definition %s has no NewMiddlewares", b.def.Name))
	}

	validateForCheckpoint[S, T](name, b.spec)

	c := &Comp[S, T, R, P, M]{
		Ports:     b.ports,
		name:      name,
		spec:      b.spec,
		resources: b.resources,
	}
	c.TickScheduler = NewTickScheduler(name, b.simulation, specFreq(b.spec))
	c.bindPorts()

	if b.def.NewState != nil {
		c.State = b.def.NewState(name, b.spec)
	}

	c.Middlewares = b.def.NewMiddlewares(c)
	c.pipeline = middlewareList(reflect.ValueOf(&c.Middlewares).Elem())

	if handlers, ok := b.simulation.GetEngine().(timing.HandlerRegistry); ok {
		handlers.RegisterHandler(name, c)
	}

	b.simulation.RegisterComponent(c)

	return c
}
