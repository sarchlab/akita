package messaging

import (
	"fmt"
	"os"
	"sort"
)

// PortOwnerBase provides an implementation of the PortOwner interface.
//
// A component owns its port topology: it declares the set of ports it has (by
// logical name, e.g. "Top") with DeclarePort, typically in its builder's
// Build. Port instances are supplied externally with AssignPort. This keeps
// the topology owned by the component while letting setup code build the port
// instances (choosing buffer sizes, implementations, etc.).
type PortOwnerBase struct {
	declared map[string]struct{}
	groups   map[string][]Port
	ports    map[string]Port
}

// NewPortOwnerBase creates a new PortOwnerBase.
func NewPortOwnerBase() *PortOwnerBase {
	return &PortOwnerBase{
		declared: make(map[string]struct{}),
		groups:   make(map[string][]Port),
		ports:    make(map[string]Port),
	}
}

// DeclarePort declares that the component has a port with the given logical
// name. The instance is supplied later with AssignPort. It panics if the name
// is already declared.
func (po *PortOwnerBase) DeclarePort(name string) {
	if _, found := po.declared[name]; found {
		panic(fmt.Sprintf("port %q already declared", name))
	}

	if _, found := po.groups[name]; found {
		panic(fmt.Sprintf("%q is already declared as a port group", name))
	}

	po.declared[name] = struct{}{}
}

// DeclarePortGroup declares that the component has a dynamically-sized group of
// ports under the given name (e.g. a switch with an arbitrary number of links).
// Members are added with AssignPortToGroup and keyed "name[0]", "name[1]", ...
// It panics if the name is already declared as a port or a group.
func (po *PortOwnerBase) DeclarePortGroup(name string) {
	if _, found := po.declared[name]; found {
		panic(fmt.Sprintf("%q is already declared as a port", name))
	}

	if _, found := po.groups[name]; found {
		panic(fmt.Sprintf("port group %q already declared", name))
	}

	po.groups[name] = nil
}

// AssignPortToGroup appends a port instance to a previously declared port group
// and returns the indexed name it is stored under ("name[i]"). It panics if the
// group was not declared.
func (po *PortOwnerBase) AssignPortToGroup(group string, port Port) string {
	members, declared := po.groups[group]
	if !declared {
		panic(fmt.Sprintf(
			"port group %q is not declared by this component", group))
	}

	name := fmt.Sprintf("%s[%d]", group, len(members))
	po.groups[group] = append(members, port)
	po.ports[name] = port

	return name
}

// NumPortsInGroup returns the number of ports currently assigned to the group.
func (po PortOwnerBase) NumPortsInGroup(group string) int {
	return len(po.groups[group])
}

// PortsInGroup returns the ports assigned to the group, in insertion order.
func (po PortOwnerBase) PortsInGroup(group string) []Port {
	return po.groups[group]
}

// AssignPort assigns a port instance to a previously declared port name. It
// panics if the name was not declared, or if a port is already assigned to it.
func (po *PortOwnerBase) AssignPort(name string, port Port) {
	if _, declared := po.declared[name]; !declared {
		panic(fmt.Sprintf("port %q is not declared by this component", name))
	}

	if _, assigned := po.ports[name]; assigned {
		panic(fmt.Sprintf("port %q already assigned", name))
	}

	po.ports[name] = port
}

// AddPort declares and assigns a port in one step. It is the legacy path for
// components that still create their own ports in Build; new components should
// declare ports (DeclarePort) and have setup code assign instances
// (AssignPort). It panics if a port with the name already exists.
func (po *PortOwnerBase) AddPort(name string, port Port) {
	if _, found := po.ports[name]; found {
		panic("port already exist")
	}

	po.declared[name] = struct{}{}
	po.ports[name] = port
}

// GetPortByName returns the port with the given logical name. It panics if the
// name is not a port of this component, or if the port was declared but no
// instance has been assigned yet.
func (po PortOwnerBase) GetPortByName(name string) Port {
	if port, assigned := po.ports[name]; assigned {
		return port
	}

	if _, declared := po.declared[name]; declared {
		panic(fmt.Sprintf(
			"port %q is declared but no instance has been assigned", name))
	}

	errMsg := fmt.Sprintf("Port %s is not available.\n", name)
	errMsg += "Available ports include:\n"

	for n := range po.ports {
		errMsg += fmt.Sprintf("\t%s\n", n)
	}

	fmt.Fprint(os.Stderr, errMsg)

	panic("port not found")
}

// Ports returns a slice of all assigned ports owned by the PortOwner, sorted
// by name.
func (po PortOwnerBase) Ports() []Port {
	portList := make([]string, 0, len(po.ports))

	for k := range po.ports {
		portList = append(portList, k)
	}

	sort.Strings(portList)

	list := make([]Port, 0, len(po.ports))

	for _, port := range portList {
		list = append(list, po.ports[port])
	}

	return list
}
