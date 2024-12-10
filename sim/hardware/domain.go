package hardware

import "github.com/sarchlab/akita/v4/sim/naming"

// Domain is a group of components that are closely connected.
type Domain struct {
	*PortOwnerBase

	name string
}

// NewDomain creates a new Domain
func NewDomain(name string) *Domain {
	naming.NameMustBeValid(name)

	d := new(Domain)

	d.name = name
	d.PortOwnerBase = NewPortOwnerBase()

	return d
}

// Name returns the name of the domain.
func (d Domain) Name() string {
	return d.name
}
