package ticking

import (
	"fmt"
	"io"
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Component is an instance of a ticking component type, built from its
// Definition. Its Spec and Resources are fixed at Build; its ports are the
// fields of Ports; its behavior is the fields of Middlewares, which Tick runs
// in declaration order.
type Component[S, T, R, P, M any] struct {
	*modeling.TickScheduler
	hooking.HookableBase

	// State is the instance's mutable runtime data. Only the component itself
	// (its NewState and middlewares) writes it.
	State T

	// Ports holds the instance's ports. The fields are fixed by P and bound to
	// the instance during Build.
	Ports P

	// Middlewares holds the instance's behavior. Tick runs the fields in
	// declaration order.
	Middlewares M

	name      string
	spec      S
	resources R
	ports     *modeling.PortTable
	pipeline  []modeling.Middleware
}

// Name returns the instance name given to Build.
func (c *Component[S, T, R, P, M]) Name() string {
	return c.name
}

// TypeName returns the component type's name: the import path of the package
// that declares it.
func (c *Component[S, T, R, P, M]) TypeName() string {
	return typeName[S]()
}

// Spec returns the instance's configuration. The returned value is a copy.
func (c *Component[S, T, R, P, M]) Spec() S {
	return c.spec
}

// Resources returns the instance's shared-resource references.
func (c *Component[S, T, R, P, M]) Resources() R {
	return c.resources
}

// Tick runs every middleware once, in the declaration order of Middlewares,
// and reports whether any of them made progress.
func (c *Component[S, T, R, P, M]) Tick() bool {
	progress := false

	for _, mw := range c.pipeline {
		if mw.Tick() {
			progress = true
		}
	}

	return progress
}

// Handle runs a tick and schedules the next one if it made progress.
func (c *Component[S, T, R, P, M]) Handle(_ timing.Event) {
	if c.Tick() {
		c.TickLater()
	}
}

// NotifyRecv wakes the instance when a port receives a message.
func (c *Component[S, T, R, P, M]) NotifyRecv(_ messaging.Port) {
	c.TickLater()
}

// NotifyPortFree wakes the instance when a port can send again.
func (c *Component[S, T, R, P, M]) NotifyPortFree(_ messaging.Port) {
	c.TickLater()
}

// GetPortByName returns the port in the Ports field with the given name, or
// member i of a port group addressed as "Group[i]".
func (c *Component[S, T, R, P, M]) GetPortByName(name string) messaging.Port {
	return c.ports.GetPortByName(name)
}

// AssignPort always panics: the ports are fixed at Build and passed with the
// builder's WithPorts.
func (c *Component[S, T, R, P, M]) AssignPort(name string, _ messaging.Port) {
	panic(fmt.Sprintf(
		"ticking: component %q: ports are fixed at Build; pass %q with WithPorts",
		c.name, name))
}

// AssignPortToGroup binds a port to the instance and appends it to the named
// port group. It returns the member's name, "Group[i]".
func (c *Component[S, T, R, P, M]) AssignPortToGroup(
	group string, port messaging.Port,
) string {
	return c.ports.AssignPortToGroup(group, port)
}

// PortsInGroup returns the members of the named port group.
func (c *Component[S, T, R, P, M]) PortsInGroup(group string) []messaging.Port {
	return c.ports.PortsInGroup(group)
}

// NumPortsInGroup returns the number of members of the named port group.
func (c *Component[S, T, R, P, M]) NumPortsInGroup(group string) int {
	return c.ports.NumPortsInGroup(group)
}

// AllPorts returns every port: the fixed ports in field order, then the
// members of each port group.
func (c *Component[S, T, R, P, M]) AllPorts() []messaging.Port {
	return c.ports.AllPorts()
}

// SaveCheckpoint writes the instance's spec hash, State, and tick-scheduler
// guard.
func (c *Component[S, T, R, P, M]) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, c.spec, c.State, c.TickScheduler)
}

// LoadCheckpoint restores the State and tick-scheduler guard after verifying
// that the saved spec hash matches this instance's.
func (c *Component[S, T, R, P, M]) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, c.spec, &c.State, c.TickScheduler)
}

// typeName returns the import path of the package that declares S, which
// identifies the component type.
func typeName[S any]() string {
	return reflect.TypeFor[S]().PkgPath()
}
