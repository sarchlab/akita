package model

import (
	"fmt"
	"os"
)

// A PortOwner is an element that can communicate with others through ports.
type PortOwner interface {
	AddPort(name string, port Port)
	GetPortByName(name string) Port
	Ports() []Port
}

// PortOwnerBase provides an implementation of the PortOwner interface.
type PortOwnerBase struct {
	ports       []Port
	portsByName map[string]Port
}

// MakePortOwnerBase creates a new PortOwnerBase
func MakePortOwnerBase() PortOwnerBase {
	return PortOwnerBase{
		ports:       make([]Port, 0),
		portsByName: make(map[string]Port),
	}
}

// AddPort adds a new port with a given name.
func (po *PortOwnerBase) AddPort(name string, port Port) {
	if _, found := po.portsByName[name]; found {
		panic("port already exist")
	}

	po.ports = append(po.ports, port)
	po.portsByName[name] = port
}

// GetPortByName returns the port according to the name of the port. This
// function panics when the given name is not found.
func (po PortOwnerBase) GetPortByName(name string) Port {
	port, found := po.portsByName[name]
	if !found {
		errMsg := fmt.Sprintf(
			"Port %s is not available.\n", name)

		errMsg += "Available ports include:\n"
		for _, port := range po.ports {
			errMsg += fmt.Sprintf("\t%s\n", port.Name())
		}

		fmt.Fprint(os.Stderr, errMsg)

		panic("port not found")
	}

	return port
}

// Ports returns a slices of all the ports owned by the PortOwner.
func (po PortOwnerBase) Ports() []Port {
	return po.ports
}
