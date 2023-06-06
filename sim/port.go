package sim

import (
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
	CanSend() bool
	Send(msg Msg) *SendError
	Retrieve(now VTimeInSec) Msg
	Peek() Msg
}

// LimitNumMsgPort is a type of port that can hold at most a certain number
// of messages.
type LimitNumMsgPort struct {
	HookableBase

	name string
	comp Component
	conn Connection

	buf          Buffer
	bufLock      sync.RWMutex
	portBusy     bool
	portBusyLock sync.RWMutex
}

// HookPosPortMsgSend marks when a message is sent out from the port.
var HookPosPortMsgSend = &HookPos{Name: "Port Msg Send"}

// HookPosPortMsgRecvd marks when an inbound message arrives at a the given port
var HookPosPortMsgRecvd = &HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieve marks when an outbound message is sent over a connection
var HookPosPortMsgRetrieve = &HookPos{Name: "Port Msg Retrieve"}

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

// CanSend checks if the port can send a message without error.
func (p *LimitNumMsgPort) CanSend() bool {
	return p.conn.CanSend(p)
}

// Send is used to send a message out from a component
func (p *LimitNumMsgPort) Send(msg Msg) *SendError {
	err := p.conn.Send(msg)

	if err == nil {
		hookCtx := HookCtx{
			Domain: p,
			Pos:    HookPosPortMsgSend,
			Item:   msg,
		}
		p.InvokeHook(hookCtx)
	}

	return err
}

// Recv is used to deliver a message to a component
func (p *LimitNumMsgPort) Recv(msg Msg) *SendError {
	p.bufLock.Lock()

	if !p.buf.CanPush() {
		p.portBusyLock.Lock()
		p.portBusy = true
		p.portBusyLock.Unlock()
		p.bufLock.Unlock()
		return NewSendError()
	}

	hookCtx := HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.buf.Push(msg)
	p.bufLock.Unlock()

	if p.comp != nil {
		p.comp.NotifyRecv(msg.Meta().RecvTime, p)
	}
	return nil
}

// Retrieve is used by the component to take a message from the incoming buffer
func (p *LimitNumMsgPort) Retrieve(now VTimeInSec) Msg {
	p.bufLock.Lock()
	defer p.bufLock.Unlock()

	item := p.buf.Pop()
	if item == nil {
		return nil
	}

	msg := item.(Msg)
	hookCtx := HookCtx{
		Domain: p,
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

	item := p.buf.Peek()
	if item == nil {
		return nil
	}

	msg := item.(Msg)
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
	p.buf = NewBuffer(name+".Buf", capacity)
	p.name = name
	return p
}

// NewLimitNumMsgPortWithExternalBuffer creates a new port that works for the
// provided component and uses the provided buffer.
func NewLimitNumMsgPortWithExternalBuffer(
	comp Component,
	buf Buffer,
	name string,
) *LimitNumMsgPort {
	NameMustBeValid(name)

	p := new(LimitNumMsgPort)
	p.comp = comp
	p.buf = buf
	p.name = name
	return p
}
