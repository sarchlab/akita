package modeling

import (
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// ComponentBase is what every component model shares: an instance's five
// structs — Spec, State, Resources, Ports, and Middlewares — and the methods
// that do not depend on how the instance is scheduled. The Component of each
// model (modeling/ticking, modeling/wakeup, and modeling/event) embeds it and
// adds its scheduling.
type ComponentBase[S, T, R, P, M any] struct {
	hooking.HookableBase

	// State is the instance's mutable runtime data. Only the component itself
	// (its NewState and middlewares) writes it.
	State T

	// Ports holds the instance's ports: the port instances the system builder
	// passed to Build, bound to this instance. They are fixed from then on.
	Ports P

	// Middlewares holds the instance's behavior. Every event the instance
	// receives goes to each field in declaration order.
	Middlewares M

	name       string
	owner      messaging.Component
	simulation timing.Simulation
	spec       S
	resources  R
}

// None is the Resources type of a component that references no shared
// objects, and the State type of one that keeps no state.
type None struct{}

// InitComponentBase sets up the ComponentBase embedded in owner, a component
// being built: it records the instance's name, simulation, Spec, and
// Resources, and binds each port to owner. A model's Build calls it first,
// then creates the State and the middlewares, and calls Register last.
func InitComponentBase[S, T, R, P, M any](
	base *ComponentBase[S, T, R, P, M],
	owner messaging.Component,
	sim timing.Simulation,
	name string,
	spec S,
	resources R,
	ports P,
) {
	base.name = name
	base.owner = owner
	base.simulation = sim
	base.spec = spec
	base.resources = resources
	base.Ports = ports
	bindPorts(owner, &base.Ports)
}

// Register registers the instance that embeds base with its simulation: each
// of its ports, then the instance as the handler of its events (when the
// engine takes handlers) and as a component. A model's Build calls it last,
// once the State and the middlewares exist, so a Build that fails registers
// nothing.
func Register[S, T, R, P, M any](base *ComponentBase[S, T, R, P, M]) {
	registerPorts(base.simulation, &base.Ports)

	if handlers, ok := base.simulation.GetEngine().(timing.HandlerRegistry); ok {
		handlers.RegisterHandler(base.name, base.owner)
	}

	base.simulation.RegisterComponent(base.owner)
}

// Name returns the instance name given to Build.
func (c *ComponentBase[S, T, R, P, M]) Name() string {
	return c.name
}

// TypeName returns the component type's name: the import path of the package
// that declares it.
func (c *ComponentBase[S, T, R, P, M]) TypeName() string {
	return reflect.TypeFor[S]().PkgPath()
}

// NewID allocates an ID, unique within the instance's simulation, for a
// message or event the instance creates.
func (c *ComponentBase[S, T, R, P, M]) NewID() uint64 {
	return c.simulation.NewID()
}

// CurrentTime returns the simulation's current time.
func (c *ComponentBase[S, T, R, P, M]) CurrentTime() timing.VTimeInPicoSec {
	return c.simulation.GetEngine().CurrentTime()
}

// Spec returns the instance's configuration. The returned value is a copy.
func (c *ComponentBase[S, T, R, P, M]) Spec() S {
	return c.spec
}

// Resources returns the instance's shared-resource references.
func (c *ComponentBase[S, T, R, P, M]) Resources() R {
	return c.resources
}
