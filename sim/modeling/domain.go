package modeling

import "github.com/sarchlab/akita/v4/sim/naming"

// Domain is a group of components that are closely connected.
type Domain struct {
	name string
	PortOwnerBase
}

// NewDomain creates a new Domain
func NewDomain(name string) *Domain {
	naming.NameMustBeValid(name)

	d := new(Domain)
	d.name = name
	d.PortOwnerBase = MakePortOwnerBase()

	return d
}

func (d *Domain) Name() string {
	return d.name
}
