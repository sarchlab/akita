package modeling

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/simulation/modeling/internal/portwalk"
	"github.com/sarchlab/akita/v5/simulation/naming"
)

// Domain is a named bundle of components. It exposes some of its internal
// components' ports at the domain boundary, so the outside world communicates
// with the domain without knowing its internal structure. Domains nest:
// components form a domain (e.g., a shader array), and domains compose into
// larger domains (e.g., a GPU), with hierarchical names following the
// "Domain.Domain.Component" convention.
//
// P declares the boundary, like a component's Ports: one messaging.Port field
// per exposed port and one []messaging.Port field per exposed group. Each
// field holds the internal port it exposes.
type Domain[P any] struct {
	// Ports holds the boundary ports: the internal ports the domain exposes.
	Ports P

	name string
}

// NewDomain creates a domain with the given hierarchical name that exposes
// the given ports. Every field of ports must be set.
func NewDomain[P any](name string, ports P) *Domain[P] {
	naming.MustBeValid(name)
	mustExposeEveryPort(name, &ports)

	return &Domain[P]{Ports: ports, name: name}
}

// Name returns the name of the domain.
func (d *Domain[P]) Name() string {
	return d.name
}

// mustExposeEveryPort checks that every port slot of the Ports struct that
// ports points to is set.
func mustExposeEveryPort(domain string, ports any) {
	portwalk.ForEach(ports, func(slot string, v reflect.Value) {
		if v.IsNil() {
			panic(fmt.Sprintf(
				"modeling: domain %q: port %s is not given", domain, slot))
		}
	})
}
