package akita

import (
	"log"
	"sync"
)

// A Port is owned by a component and is used to plugin connections
type Port interface {
	// Embed interface
	Named
	Hookable

	SetConnection(conn Connection)
	Component() Component

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
	HookableBase

	name string
	comp Component
	conn Connection

	buf          []Msg
	bufLock      sync.RWMutex
	bufCapacity  int
	portBusy     bool
	portBusyLock sync.RWMutex
}

// HookPosPortMsgRecvd marks when an inbound message arrives at a the given port
var HookPosPortMsgRecvd = &HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieve marks when an outbound message is sent over a connection
var HookPosPortMsgRetrieve = &HookPos{Name: "Port Msg  Retrieve"}

// SetConnection sets which connection plugged in to this port.
func (p *LimitNumMsgPort) SetConnection(conn Connection) {
	p.conn = conn
}

// Component returns the owner component of the port.
func (p *LimitNumMsgPort) Component() Component {
	return p.comp
}

// Name returns the name of the port.
func (p *LimitNumMsgPort) Name() string {
	return p.name
}

// Send is used to send a message out from a component
func (p *LimitNumMsgPort) Send(msg Msg) *SendError {
	err := p.conn.Send(msg)
	return err
}

// Recv is used to deliver a message to a component
func (p *LimitNumMsgPort) Recv(msg Msg) *SendError {
	p.bufLock.Lock()
	defer p.bufLock.Unlock()

	if len(p.buf) >= p.bufCapacity {
		p.portBusyLock.Lock()
		p.portBusy = true
		p.portBusyLock.Unlock()
		return NewSendError()
	}

	hookCtx := HookCtx{
		Domain: p,
		Now:    msg.Meta().RecvTime,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.buf = append(p.buf, msg)
	if p.comp != nil {
		p.comp.NotifyRecv(msg.Meta().RecvTime, p)
	}
	return nil
}

// Retrieve is used by the component to take a message from the incoming buffer
func (p *LimitNumMsgPort) Retrieve(now VTimeInSec) Msg {
	p.bufLock.Lock()
	defer p.bufLock.Unlock()

	if len(p.buf) == 0 {
		return nil
	}

	msg := p.buf[0]
	p.buf = p.buf[1:]
	hookCtx := HookCtx{
		Domain: p,
		Now:    now,
		Pos:    HookPosPortMsgRetrieve,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.portBusyLock.Lock()
	if p.portBusy {
		p.portBusy = false
		p.conn.NotifyAvailable(now, p)
	}
	p.portBusyLock.Unlock()

	return msg
}

// Peek returns the first message in the port without removing it.
func (p *LimitNumMsgPort) Peek() Msg {
	p.bufLock.RLock()
	defer p.bufLock.RUnlock()

	if len(p.buf) == 0 {
		return nil
	}

	msg := p.buf[0]
	return msg
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *LimitNumMsgPort) NotifyAvailable(now VTimeInSec) {
	if p.comp != nil {
		p.comp.NotifyPortFree(now, p)
	}
}

// NewLimitNumMsgPort creates a new port that works for the provided component
func NewLimitNumMsgPort(
	comp Component,
	capacity int,
	name string,
) *LimitNumMsgPort {
	p := new(LimitNumMsgPort)
	p.comp = comp
	p.bufCapacity = capacity
	p.name = name
	return p
}
