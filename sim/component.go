package sim

import (
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
	PortOwner

	NotifyRecv(now VTimeInSec, port Port)
	NotifyPortFree(now VTimeInSec, port Port)
}

func (c Component) SetConnection(conn Connection) {
	//TODO implement me
	panic("implement me")
}

func (c Component) Component() Component {
	//TODO implement me
	panic("implement me")
}

func (c Component) Recv(msg Msg) *SendError {
	//TODO implement me
	panic("implement me")
}

func (c Component) NotifyAvailable(now VTimeInSec) {
	//TODO implement me
	panic("implement me")
}

func (c Component) CanSend() bool {
	//TODO implement me
	panic("implement me")
}

func (c Component) Send(msg Msg) *SendError {
	//TODO implement me
	panic("implement me")
}

func (c Component) Retrieve(now VTimeInSec) Msg {
	//TODO implement me
	panic("implement me")
}

func (c Component) Peek() Msg {
	//TODO implement me
	panic("implement me")
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
