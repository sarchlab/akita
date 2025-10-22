// Package comm defines communication primitives for Akita V5.
package comm

import (
	"reflect"
)

// PortAddr identifies a port in the simulation topology.
type PortAddr string

// Msg describes the metadata contract shared by all messages in the
// communication layer. Implementations should expose their identifying fields
// via simple getters.
type Msg interface {
	ID() string
	Src() PortAddr
	Dst() PortAddr
	TrafficClass() string
	TrafficBytes() int
}

func typeName(v any) string {
	if v == nil {
		return ""
	}

	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t.String()
}
