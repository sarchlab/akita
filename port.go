package akita

import (
	"log"
	"sync"
)

// A Port is owned by a component and is used to plugin connections
type Port interface {
	SetConnection(conn Connection)
	Component() Component

	// Embed interface
	Named
	Hookable

	// For connection
	Recv(msg Msg) *SendError
	NotifyAvailable(now VTimeInSec)

	// For component
	Send(msg Msg) *SendError
	Retrieve(now VTimeInSec) Msg
	Peek() Msg
}

// PortEndSimulationChecker checks if the port buffer is empty at the end of
// the simulation. If the port is not empty, there is something wrong in the
// simulation.
type PortEndSimulationChecker struct {
	Port Port
}

// Handle checks if the port is empty or not.
func (c *PortEndSimulationChecker) Handle(e Event) error {
	if c.Port.Peek() != nil {
		log.Panic("port is not free")
	}
	return nil
}

// LimitNumMsgPort is a type of port that can hold at most a certain number
// of messages.
type LimitNumMsgPort struct {
	sync.Mutex

	Buf         []Msg
	BufCapacity int
	name        string
	PortBusy    bool

	Conn     Connection
	ConnBusy bool

	*HookableBase

	Comp Component
}

// HookPosPortMsgRecvd marks when an inbound message arrives at a the given port
var HookPosPortMsgRecvd = &HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieve marks when an outbound message is sent over a connection
var HookPosPortMsgRetrieve = &HookPos{Name: "Port Msg  Retrieve"}

// SetConnection sets which connection plugged in to this port.
func (p *LimitNumMsgPort) SetConnection(conn Connection) {
	p.Conn = conn
}

// Component returns the owner component of the port.
func (p *LimitNumMsgPort) Component() Component {
	return p.Comp
}

// Name returns the name of the port.
func (p *LimitNumMsgPort) Name() string {
	return p.name
}

// Send is used to send a message out from a component
func (p *LimitNumMsgPort) Send(msg Msg) *SendError {

	err := p.Conn.Send(msg)
	if err != nil {
		p.Lock()
		p.ConnBusy = true
		p.Unlock()
	}

	return err
}

// Recv is used to deliver a message to a component
func (p *LimitNumMsgPort) Recv(msg Msg) *SendError {
	p.Lock()
	if len(p.Buf) >= p.BufCapacity {
		p.PortBusy = true
		p.Unlock()
		return NewSendError()
	}

	now = msg.Time()
	hookCtx := HookCtx{
		Domain: p,
		Now:    now,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(&hookCtx)

	p.Buf = append(p.Buf, msg)
	p.Unlock()
	if p.Comp != nil {
		p.Comp.NotifyRecv(msg.Meta().RecvTime, p)
	}
	return nil
}

// Retrieve is used by the component to take a message from the incoming buffer
func (p *LimitNumMsgPort) Retrieve(now VTimeInSec) Msg {
	p.Lock()
	if len(p.Buf) == 0 {
		p.Unlock()
		return nil
	}

	msg := p.Buf[0]
	p.Buf = p.Buf[1:]

	now = msg.Time()
	hookCtx := HookCtx{
		Domain: p,
		Now:    now,
		Pos:    HookPosPortMsgRetrieve,
		Item:   msg,
	}
	p.InvokeHook(&hookCtx)

	if p.PortBusy == true {
		p.PortBusy = false
		p.Unlock()
		p.Conn.NotifyAvailable(now, p)
		return msg
	}

	p.Unlock()
	return msg
}

// Peek returns the first message in the port without removing it.
func (p *LimitNumMsgPort) Peek() Msg {
	p.Lock()
	if len(p.Buf) == 0 {
		p.Unlock()
		return nil
	}
	msg := p.Buf[0]
	p.Unlock()
	return msg
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *LimitNumMsgPort) NotifyAvailable(now VTimeInSec) {
	p.Lock()
	p.ConnBusy = false
	p.Unlock()

	if p.Comp != nil {
		p.Comp.NotifyPortFree(now, p)
	}
}

// NewLimitNumMsgPort creates a new port that works for the provided component
func NewLimitNumMsgPort(comp Component, capacity int, name string) *LimitNumMsgPort {
	p := new(LimitNumMsgPort)
	p.Comp = comp
	p.BufCapacity = capacity
	p.name = name
	return p
}
