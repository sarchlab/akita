package akita

import (
	"sync"
)

// A Named object is an object that has a name
type Named interface {
	Name() string
}

// A Component is a element that is being simulated in Akita.
type Component interface {
	Named
	Handler
	Hookable

	NotifyRecv(now VTimeInSec, port Port)
	NotifyPortFree(now VTimeInSec, port Port)
}

// ComponentBase provides some functions that other component can use
type ComponentBase struct {
	HookableBase
	sync.Mutex
	name string
}

// NewComponentBase creates a new ComponentBase
func NewComponentBase(name string) *ComponentBase {
	c := new(ComponentBase)
	c.name = name
	// c.HookableBase = NewHookableBase()
	return c
}

// Name returns the name of the BasicComponent
func (c *ComponentBase) Name() string {
	return c.name
}
