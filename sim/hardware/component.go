package hardware

import (
	"sync"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/naming"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A named object is an object that has a name.
type named interface {
	Name() string
}

// A Component is a element that is being simulated in Akita.
type Component interface {
	named
	timing.Handler
	hooking.Hookable
	PortOwner

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}

// ComponentBase provides some functions that other component can use.
type ComponentBase struct {
	sync.Mutex
	hooking.HookableBase
	*PortOwnerBase

	name string
}

// NewComponentBase creates a new ComponentBase
func NewComponentBase(name string) *ComponentBase {
	naming.NameMustBeValid(name)

	c := new(ComponentBase)
	c.name = name
	c.PortOwnerBase = NewPortOwnerBase()

	return c
}

// Name returns the name of the BasicComponent
func (c *ComponentBase) Name() string {
	return c.name
}
