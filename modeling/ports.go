package modeling

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

var (
	portType      = reflect.TypeFor[messaging.Port]()
	portSliceType = reflect.TypeFor[[]messaging.Port]()
)

// forEachPort calls fn for every port slot of the Ports struct that ports
// points to: one slot per messaging.Port field, named after the field, and
// one per member of a []messaging.Port field (a port group), named
// "<field>[i]". It panics unless every field is an exported messaging.Port or
// []messaging.Port.
func forEachPort(ports any, fn func(slot string, v reflect.Value)) {
	v := reflect.ValueOf(ports).Elem()
	t := v.Type()

	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || (f.Type != portType && f.Type != portSliceType) {
			panic(fmt.Sprintf(
				"modeling: Ports field %s.%s must be an exported messaging.Port "+
					"or []messaging.Port", t, f.Name))
		}

		field := v.Field(i)
		if f.Type == portType {
			fn(f.Name, field)
			continue
		}

		for j := range field.Len() {
			fn(fmt.Sprintf("%s[%d]", f.Name, j), field.Index(j))
		}
	}
}

// bindPorts binds every port in the Ports struct that ports points to to
// owner. Every port must be given and carry its full name: "<owner>.<field>"
// for a port, and "<owner>.<field>[i]" for member i of a group.
func bindPorts(owner Component, ports any) {
	forEachPort(ports, func(slot string, v reflect.Value) {
		if v.IsNil() {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is not given; pass it with WithPorts",
				owner.Name(), slot))
		}

		port := v.Interface().(messaging.Port)

		if want := owner.Name() + "." + slot; port.Name() != want {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is named %q, want %q",
				owner.Name(), slot, port.Name(), want))
		}

		if other := port.Owner(); other != nil && other != owner {
			panic(fmt.Sprintf(
				"modeling: component %q: port %q already belongs to %s",
				owner.Name(), port.Name(), ownerName(other)))
		}

		port.SetOwner(owner)
	})
}

// ownerName names a port's owner for an error message. An owner need not be
// named.
func ownerName(owner messaging.PortOwner) string {
	if named, ok := owner.(naming.Named); ok {
		return fmt.Sprintf("%q", named.Name())
	}

	return fmt.Sprintf("an unnamed %T", owner)
}

// registerPorts registers every port in the Ports struct that ports points to
// with the simulation.
func registerPorts(sim timing.Simulation, ports any) {
	forEachPort(ports, func(_ string, v reflect.Value) {
		sim.RegisterPort(v.Interface().(messaging.Port))
	})
}
