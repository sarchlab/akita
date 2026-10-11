// Package portwalk walks the port slots of a Ports struct. The component
// models share it.
package portwalk

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

var (
	portType      = reflect.TypeFor[messaging.Port]()
	portSliceType = reflect.TypeFor[[]messaging.Port]()
)

// ForEach calls fn for every port slot of the Ports struct that ports points
// to: one slot per messaging.Port field, named after the field, and one per
// member of a []messaging.Port field (a port group), named "<field>[i]". It
// panics unless every field is an exported messaging.Port or
// []messaging.Port.
func ForEach(ports any, fn func(slot string, v reflect.Value)) {
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
