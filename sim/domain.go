package sim

import (
	"fmt"
	"os"
)

// Domain is a group of components that are closely connected.
type Domain struct {
	name  string
	ports map[string]Port
}

// Name returns the name of the domain.
func (d Domain) Name() string {
	return d.name
}

// RegisterPort registers a port that can be accessed outside the domain.
func (d *Domain) RegisterPort(name string, p Port) {
	if _, found := d.ports[name]; found {
		panic("port already exist")
	}

	d.ports[name] = p
}

// GetPortByName returns the name of the port.
func (d *Domain) GetPortByName(name string) Port {
	port, found := d.ports[name]
	if !found {
		errMsg := fmt.Sprintf(
			"Port %s is not available on component %s.\n", name, d.name)
		errMsg += "Available ports include:\n"
		for n := range d.ports {
			errMsg += fmt.Sprintf("\t%s\n", n)
		}
		fmt.Fprint(os.Stderr, errMsg)

		panic("port not found")
	}

	return port
}
