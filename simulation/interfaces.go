package simulation

import "reflect"

// Entity is the abstract base interface for every registered runtime object.
// Components, ports, connections, and resources all satisfy it. It is the
// common vocabulary the global state manager uses to track entities and resolve
// them by name; the concrete kinds extend it with their own capabilities.
type Entity interface {
	Name() string
}

// Component is the minimal component contract the simulation runtime needs.
// Concrete messaging components satisfy this without the simulation package
// depending on messaging.
type Component interface {
	Entity
}

// Port is the minimal port contract the simulation runtime needs.
// Concrete messaging ports satisfy this without the simulation package
// depending on messaging.
type Port interface {
	Entity
	NumIncoming() int
	NumOutgoing() int
}

// Connection is the minimal connection contract the simulation runtime needs.
// Concrete messaging connections satisfy this without the simulation package
// depending on messaging.
type Connection interface {
	Entity
}

// PortOwner is implemented by simulation-native components that expose ports.
//
// Existing messaging components return []messaging.Port from Ports, which is
// not assignable to []simulation.Port in Go. RegisterComponent also supports
// those components through reflection in componentPorts.
type PortOwner interface {
	Ports() []Port
}

func componentPorts(c Component) []Port {
	if owner, ok := c.(PortOwner); ok {
		return owner.Ports()
	}

	method := reflect.ValueOf(c).MethodByName("Ports")
	if !method.IsValid() {
		return nil
	}

	methodType := method.Type()
	if methodType.NumIn() != 0 ||
		methodType.NumOut() != 1 ||
		methodType.Out(0).Kind() != reflect.Slice {
		panic("component " + c.Name() +
			" Ports method must take no arguments and return one slice")
	}

	values := method.Call(nil)
	portsValue := values[0]
	ports := make([]Port, 0, portsValue.Len())

	for i := 0; i < portsValue.Len(); i++ {
		port, ok := portsValue.Index(i).Interface().(Port)
		if !ok {
			panic("component " + c.Name() +
				" Ports method returned a non-simulation port")
		}

		ports = append(ports, port)
	}

	return ports
}
