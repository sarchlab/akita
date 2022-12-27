package sim

// Domain is a group of components that are closely connected.
type Domain struct {
	*PortOwnerBase

	name string
}

// NewDomain creates a new Domain
func NewDomain(name string) *Domain {
	NameMustBeValid(name)

	d := new(Domain)

	d.name = name
	d.PortOwnerBase = NewPortOwnerBase()

	return d
}

// Name returns the name of the domain.
func (d Domain) Name() string {
	return d.name
}
