package modeling

import "github.com/sarchlab/akita/v4/sim/naming"

// Domain is a group of components that are closely connected.
type Domain struct {
	naming.NamedBase
	PortOwnerBase
}

// NewDomain creates a new Domain
func NewDomain(name string) *Domain {
	naming.NameMustBeValid(name)

	d := new(Domain)
	d.NamedBase = naming.MakeNamedBase(name)
	d.PortOwnerBase = MakePortOwnerBase()

	return d
}
