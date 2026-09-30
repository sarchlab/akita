package modeling

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition defines a component type by five fixed-shape structs:
//
//   - S, the Spec: scalar configuration, supplied by the caller;
//   - T, the State: mutable runtime data, created by NewState;
//   - R, the Resources: references to shared objects, supplied by the caller;
//   - P, the Ports: one messaging.Port field per port and one
//     []messaging.Port field per port group, supplied by the caller;
//   - M, the Middlewares: one field per middleware, created by NewMiddlewares
//     and run in field order on every tick.
//
// Declare it as a package-level var named Definition in the component's
// package and build components with Definition.Builder(). Name and
// DefaultSpec must be statically evaluable, as for ComponentDef; NewState and
// NewMiddlewares name functions.
//
// Definition is the prototype of the component model proposed in #492. It
// takes the name ComponentDef once the legacy API is removed.
type Definition[S, T, R, P, M any] struct {
	// Name is the component's display name, e.g. "ReorderBuffer".
	Name string

	// DefaultSpec is the component's default configuration. Spec fields are
	// scalars, so reading it yields an independent copy.
	DefaultSpec S

	// NewState returns the initial State of a component with the given name
	// and Spec. A nil NewState starts from the zero State.
	NewState func(name string, spec S) T

	// NewMiddlewares returns the component's middlewares. Build calls it once,
	// after the ports are bound and the State is set.
	NewMiddlewares func(c *Comp[S, T, R, P, M]) M
}

// Builder returns a builder for components of this definition, starting from
// DefaultSpec.
func (d Definition[S, T, R, P, M]) Builder() CompBuilder[S, T, R, P, M] {
	return CompBuilder[S, T, R, P, M]{def: d, spec: d.DefaultSpec}
}

// Comp is a component built from a Definition. Its Spec and Resources are
// fixed at Build; its ports are the fields of Ports; its behavior is the
// fields of Middlewares, which Tick runs in declaration order.
//
// Comp is the prototype of the component model proposed in #492. It takes the
// name Component once the legacy API is removed.
type Comp[S, T, R, P, M any] struct {
	*TickScheduler
	hooking.HookableBase

	// State is the component's mutable runtime data. Only the component itself
	// (its NewState and middlewares) writes it.
	State T

	// Ports holds the component's ports. The fields are fixed by P and bound to
	// the component during Build.
	Ports P

	// Middlewares holds the component's behavior. Tick runs the fields in
	// declaration order.
	Middlewares M

	name      string
	spec      S
	resources R
	pipeline  []Middleware
}

// Name returns the component's name.
func (c *Comp[S, T, R, P, M]) Name() string {
	return c.name
}

// Spec returns the component's configuration. The returned value is a copy.
func (c *Comp[S, T, R, P, M]) Spec() S {
	return c.spec
}

// Resources returns the component's shared-resource references.
func (c *Comp[S, T, R, P, M]) Resources() R {
	return c.resources
}

// Tick runs every middleware once, in the declaration order of Middlewares,
// and reports whether any of them made progress.
func (c *Comp[S, T, R, P, M]) Tick() bool {
	progress := false

	for _, mw := range c.pipeline {
		if mw.Tick() {
			progress = true
		}
	}

	return progress
}

// Handle runs a tick and schedules the next one if it made progress.
func (c *Comp[S, T, R, P, M]) Handle(_ timing.Event) {
	if c.Tick() {
		c.TickLater()
	}
}

// NotifyRecv wakes the component when a port receives a message.
func (c *Comp[S, T, R, P, M]) NotifyRecv(_ messaging.Port) {
	c.TickLater()
}

// NotifyPortFree wakes the component when a port can send again.
func (c *Comp[S, T, R, P, M]) NotifyPortFree(_ messaging.Port) {
	c.TickLater()
}

// GetPortByName returns the port stored in the Ports field with the given
// name, or member i of a port group addressed as "Group[i]".
func (c *Comp[S, T, R, P, M]) GetPortByName(name string) messaging.Port {
	ports := c.portsValue()
	layout := portLayoutOf(ports.Type())

	if f, ok := layout.byName[name]; ok && !f.group {
		return ports.Field(f.index).Interface().(messaging.Port)
	}

	if group, idx, ok := parseGroupMember(name); ok {
		if f, found := layout.byName[group]; found && f.group {
			members := ports.Field(f.index)
			if idx < members.Len() {
				return members.Index(idx).Interface().(messaging.Port)
			}
		}
	}

	panic(fmt.Sprintf("modeling: component %q has no port %q; ports are %s",
		c.name, name, strings.Join(layout.names(), ", ")))
}

// AssignPort always panics: a Comp's ports are fixed at Build and passed with
// the builder's WithPorts.
func (c *Comp[S, T, R, P, M]) AssignPort(name string, _ messaging.Port) {
	panic(fmt.Sprintf(
		"modeling: component %q: ports are fixed at Build; pass %q with WithPorts",
		c.name, name))
}

// AssignPortToGroup binds a port to the component and appends it to the named
// port group. It returns the member's name, "Group[i]".
func (c *Comp[S, T, R, P, M]) AssignPortToGroup(
	group string, port messaging.Port,
) string {
	members := c.groupValue(group)
	name := group + "[" + strconv.Itoa(members.Len()) + "]"

	c.bindPort(port, "")
	members.Set(reflect.Append(members, reflect.ValueOf(port)))

	return name
}

// PortsInGroup returns the members of the named port group, in the order they
// were added.
func (c *Comp[S, T, R, P, M]) PortsInGroup(group string) []messaging.Port {
	members := c.groupValue(group)
	out := make([]messaging.Port, members.Len())
	for i := range out {
		out[i] = members.Index(i).Interface().(messaging.Port)
	}

	return out
}

// NumPortsInGroup returns the number of members of the named port group.
func (c *Comp[S, T, R, P, M]) NumPortsInGroup(group string) int {
	return c.groupValue(group).Len()
}

// AllPorts returns every port of the component: the fixed ports in field
// order, then the members of each port group.
func (c *Comp[S, T, R, P, M]) AllPorts() []messaging.Port {
	ports := c.portsValue()
	layout := portLayoutOf(ports.Type())

	var out []messaging.Port
	for _, f := range layout.fields {
		v := ports.Field(f.index)
		if !f.group {
			if !v.IsNil() {
				out = append(out, v.Interface().(messaging.Port))
			}
			continue
		}

		for i := 0; i < v.Len(); i++ {
			out = append(out, v.Index(i).Interface().(messaging.Port))
		}
	}

	return out
}

// SaveCheckpoint writes the component's spec hash, State, and scheduler guard.
func (c *Comp[S, T, R, P, M]) SaveCheckpoint(w io.Writer) error {
	return saveCheckpoint(w, c.spec, c.State, c.TickScheduler)
}

// LoadCheckpoint restores State and the scheduler guard after verifying that
// the saved spec hash matches the rebuilt component's.
func (c *Comp[S, T, R, P, M]) LoadCheckpoint(r io.Reader) error {
	return loadCheckpoint(r, c.spec, &c.State, c.TickScheduler)
}

// bindPorts binds every port given to the builder: each fixed port must be
// present and named "<component>.<field>".
func (c *Comp[S, T, R, P, M]) bindPorts() {
	ports := c.portsValue()

	for _, f := range portLayoutOf(ports.Type()).fields {
		v := ports.Field(f.index)

		if f.group {
			for i := 0; i < v.Len(); i++ {
				c.bindPort(v.Index(i).Interface().(messaging.Port), "")
			}
			continue
		}

		if v.IsNil() {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is not given; pass it with WithPorts",
				c.name, f.name))
		}

		c.bindPort(v.Interface().(messaging.Port), f.name)
	}
}

// bindPort makes the component the port's owner. A non-empty field is the
// Ports field the port fills, whose full name the port must carry.
func (c *Comp[S, T, R, P, M]) bindPort(port messaging.Port, field string) {
	if field != "" {
		if want := c.name + "." + field; port.Name() != want {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is named %q, want %q",
				c.name, field, port.Name(), want))
		}
	}

	if owner := port.Component(); owner != nil && owner != messaging.Component(c) {
		panic(fmt.Sprintf(
			"modeling: component %q: port %q already belongs to %q",
			c.name, port.Name(), owner.Name()))
	}

	port.SetComponent(c)
}

func (c *Comp[S, T, R, P, M]) portsValue() reflect.Value {
	return reflect.ValueOf(&c.Ports).Elem()
}

// groupValue returns the settable slice of the named port group.
func (c *Comp[S, T, R, P, M]) groupValue(group string) reflect.Value {
	ports := c.portsValue()

	f, ok := portLayoutOf(ports.Type()).byName[group]
	if !ok || !f.group {
		panic(fmt.Sprintf("modeling: component %q has no port group %q",
			c.name, group))
	}

	return ports.Field(f.index)
}

// parseGroupMember splits "Group[i]" into the group name and index.
func parseGroupMember(name string) (string, int, bool) {
	open := strings.IndexByte(name, '[')
	if open <= 0 || !strings.HasSuffix(name, "]") {
		return "", 0, false
	}

	idx, err := strconv.Atoi(name[open+1 : len(name)-1])
	if err != nil || idx < 0 {
		return "", 0, false
	}

	return name[:open], idx, true
}
