package core

import (
	"log"
	"sync"
)

// A Named object is an object that has a name
type Named interface {
	Name() string
}

// A Component is a element that is being simulated in Yaotsu.
type Component interface {
	// A Component must have a name to distinguish itself
	Named

	// A Component can connect to the outside world
	Connectable

	// A Component can handle events
	Handler

	// A Component should be hookable
	Hookable
}

// ComponentBase provides some functions that other component can use
type ComponentBase struct {
	*HookableBase
	sync.Mutex

	name        string
	connections map[string]Connection
	ports       map[string]bool
}

// NewComponentBase creates a new ComponentBase
func NewComponentBase(name string) *ComponentBase {
	c := new(ComponentBase)
	c.name = name
	c.HookableBase = NewHookableBase()
	c.connections = make(map[string]Connection)
	c.ports = make(map[string]bool)
	return c
}

// Name returns the name of the BasicComponent
func (c *ComponentBase) Name() string {
	return c.name
}

// Connect of BasicComponent associate a connection with a port of the component
func (c *ComponentBase) Connect(portName string, conn Connection) {
	if _, ok := c.ports[portName]; !ok {
		log.Panicf("Component " + c.Name() + " does not have port " + portName)
	}

	c.connections[portName] = conn
}

// GetConnection returns the connection by the port name
func (c *ComponentBase) GetConnection(portName string) Connection {
	return c.connections[portName]
}

// Disconnect removes the association between the port name and the connection
func (c *ComponentBase) Disconnect(portName string) {
	if _, ok := c.ports[portName]; !ok {
		log.Panicf("Component " + c.Name() + " does not have port " + portName)
	}

	if _, ok := c.connections[portName]; !ok {
		log.Panic("Port " + portName + "is not connected")
	}

	delete(c.connections, portName)
}

// AddPort register a port name to the component.
//
// After defining the port names, all the connections must specify which port
// that the connection is connecting to. When the component need to send
// requests out, it need first get the connection by the port name, and then
// send the request over the connection.
func (c *ComponentBase) AddPort(name string) {
	if name == "" {
		log.Panic("cannot use empty string as port name")
	}

	if _, ok := c.ports[name]; ok {
		log.Panic("cannot duplicate port name " + name)
	}

	c.ports[name] = true
}
