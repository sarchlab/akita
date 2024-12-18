package naming

import (
	"reflect"

	"github.com/sarchlab/akita/v4/sim/serialization"
)

func init() {
	serialization.RegisterType(reflect.TypeOf((*Named)(nil)).Elem())
}

// Named describes an object that has a name.
type Named interface {
	// Name returns the name of the object.
	Name() string
}

// NamedBase is a base implementation of Named.
type NamedBase struct {
	name string
}

func (b *NamedBase) Name() string {
	return b.name
}

// MakeNamedBase creates a new NamedBase
func MakeNamedBase(name string) NamedBase {
	return NamedBase{name: name}
}
