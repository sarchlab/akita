package conn

import (
	"errors"
)

// A Named object is an object that has a name
type Named interface {
	Name() string
}

// A Component is a element that is being simulated in Yaotsu.
type Component interface {
	Named
	Connectable

	AddPort(name string) error
}

// BasicComponent provides some functions that other component can use
type BasicComponent struct {
	name        string
	connections map[string]Connection
	Ports       map[string]bool
}

// NewBasicComponent creates a new basic component
func NewBasicComponent(name string) *BasicComponent {
	return &BasicComponent{
		name,
		make(map[string]Connection),
		make(map[string]bool),
	}
}

// Name returns the name of the BasicComponent
func (c *BasicComponent) Name() string {
	return c.name
}

// Connect of BasicComponent associate a connection with a port of the component
func (c *BasicComponent) Connect(portName string, conn Connection) error {
	if _, ok := c.Ports[portName]; !ok {
		return errors.New("Component " + c.Name() + " does not have port " +
			portName)
	}

	c.connections[portName] = conn
	return nil
}

// GetConnection returns the connection by the port name
func (c *BasicComponent) GetConnection(portName string) Connection {
	return c.connections[portName]
}

// Disconnect removes the association between the port name and the connection
func (c *BasicComponent) Disconnect(portName string) error {
	if _, ok := c.Ports[portName]; !ok {
		return errors.New("Component " + c.Name() + " does not have port " +
			portName)
	}

	if _, ok := c.connections[portName]; !ok {
		return errors.New("Port " + portName + "is not connected")
	}

	delete(c.connections, portName)
	return nil
}

// AddPort register a port name to the component.
//
// After defining the port names, all the connections must specify which port
// that the connection is connecting to. When the component need to send
// requests out, it need first get the connection by the port name, and then
// send the request over the connection.
func (c *BasicComponent) AddPort(name string) error {
	if name == "" {
		return errors.New("cannot use empty string as port name")
	}

	if _, ok := c.Ports[name]; ok {
		return errors.New("cannot duplicate port name " + name)
	}

	c.Ports[name] = true
	return nil
}
