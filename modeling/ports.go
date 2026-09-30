package modeling

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

var (
	portType      = reflect.TypeFor[messaging.Port]()
	portSliceType = reflect.TypeFor[[]messaging.Port]()
)

// bindPorts binds every port in a Ports struct to owner and registers it with
// the simulation. ports points to the struct, which has one messaging.Port
// field per port and one []messaging.Port field per port group. Every port
// must be given and carry its full name: "<owner>.<field>" for a port, and
// "<owner>.<field>[i]" for member i of a group. No port is added later.
func bindPorts(owner messaging.Component, sim timing.Simulation, ports any) {
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
			bindPort(owner, sim, field, f.Name)
			continue
		}

		for j := range field.Len() {
			bindPort(owner, sim, field.Index(j), fmt.Sprintf("%s[%d]", f.Name, j))
		}
	}
}

// bindPort binds the port held in v, which fills the named slot of owner's
// Ports.
func bindPort(
	owner messaging.Component, sim timing.Simulation, v reflect.Value, slot string,
) {
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

	if other := port.Component(); other != nil && other != owner {
		panic(fmt.Sprintf(
			"modeling: component %q: port %q already belongs to %q",
			owner.Name(), port.Name(), other.Name()))
	}

	port.SetComponent(owner)
	sim.RegisterPort(port)
}
