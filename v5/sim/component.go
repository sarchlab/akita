package sim

import (
	"sync"
)

// A Named object is an object that has a name.
type Named interface {
	Name() string
}

// A Component is an element being simulated in Akita.
//
// Component is the unifying interface for all simulation elements that
// own ports and can be notified of port activity. Event handling
// (Handler interface) is intentionally NOT part of Component —
// event dispatch is handled by the Engine via Event.Handler().
type Component interface {
	Named
	Hookable
	PortOwner

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}

// ComponentBase provides some functions that other component can use.
type ComponentBase struct {
	sync.Mutex
	HookableBase
	*PortOwnerBase

	name string
}

// NewComponentBase creates a new ComponentBase
func NewComponentBase(name string) *ComponentBase {
	NameMustBeValid(name)

	c := new(ComponentBase)
	c.name = name
	c.PortOwnerBase = NewPortOwnerBase()

	return c
}

// Name returns the name of the BasicComponent
func (c *ComponentBase) Name() string {
	return c.name
}
