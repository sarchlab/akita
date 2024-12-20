package modeling

import (
	"sync"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/naming"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A Component is a element that is being simulated in Akita.
type Component interface {
	naming.Named
	timing.Handler
	hooking.Hookable
	PortOwner

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}

// ComponentBase provides some functions that other component can use.
type ComponentBase struct {
	name string
	sync.Mutex
	hooking.HookableBase
	PortOwnerBase
}

// NewComponentBase creates a new ComponentBase
func NewComponentBase(name string) *ComponentBase {
	naming.NameMustBeValid(name)

	c := new(ComponentBase)
	c.name = name
	c.PortOwnerBase = MakePortOwnerBase()

	return c
}

func (c *ComponentBase) Name() string {
	return c.name
}
