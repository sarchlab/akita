package sim

import (
	"fmt"
	"os"
	"sync"
)

// A Named object is an object that has a name.
type Named interface {
	Name() string
}

// A Component is a element that is being simulated in Akita.
type Component interface {
	Named
	Handler
	Hookable

	GetPortByName(name string) Port
	NotifyRecv(now VTimeInSec, port Port)
	NotifyPortFree(now VTimeInSec, port Port)
}

// ComponentBase provides some functions that other component can use.
type ComponentBase struct {
	HookableBase
	sync.Mutex
	name  string
	ports map[string]Port
}

// NewComponentBase creates a new ComponentBase
func NewComponentBase(name string) *ComponentBase {
	c := new(ComponentBase)
	c.name = name
	c.ports = make(map[string]Port)
	return c
}

// Name returns the name of the BasicComponent
func (c *ComponentBase) Name() string {
	return c.name
}

// GetPortByName returns the port by the name of the port.
func (c *ComponentBase) GetPortByName(name string) Port {
	port, found := c.ports[name]
	if !found {
		errMsg := fmt.Sprintf(
			"Port %s is not available on component %s.\n", name, c.name)
		errMsg += "Available ports include:\n"
		for n := range c.ports {
			errMsg += fmt.Sprintf("\t%s\n", n)
		}
		fmt.Fprint(os.Stderr, errMsg)

		panic("port not found")
	}

	return port
}
