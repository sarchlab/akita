package sim

import (
	"fmt"
	"sync"
)

// HookPosPortMsgSend marks when a message is sent out from the port.
var HookPosPortMsgSend = &HookPos{Name: "Port Msg Send"}

// HookPosPortMsgRecvd marks when an inbound message arrives at a the given port
var HookPosPortMsgRecvd = &HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieveIncoming marks when an inbound message is retrieved
// from the incoming buffer.
var HookPosPortMsgRetrieveIncoming = &HookPos{
	Name: "Port Msg Retrieve Incoming",
}

// HookPosPortMsgRetrieveOutgoing marks when an outbound message is retrieved
// from the outgoing buffer.
var HookPosPortMsgRetrieveOutgoing = &HookPos{
	Name: "Port Msg Retrieve Outgoing",
}

// A RemotePort is a string that refers to another port.
type RemotePort string

// A Port is owned by a component and is used to plugin connections
type Port interface {
	Named
	Hookable

	AsRemote() RemotePort

	SetConnection(conn Connection)
	Component() Component

	// For connection
	Deliver(msg Msg) *SendError
	NotifyAvailable()
	RetrieveOutgoing() Msg
	PeekOutgoing() Msg

	// For component
	CanSend() bool
	Send(msg Msg) *SendError
	RetrieveIncoming() Msg
	PeekIncoming() Msg
}

// DefaultPort implements the port interface.
type defaultPort struct {
	HookableBase

	lock sync.Mutex
	name string
	comp Component
	conn Connection

	incomingBuf Buffer
	outgoingBuf Buffer
}

// AsRemote returns the remote port name.
func (p *defaultPort) AsRemote() RemotePort {
	return RemotePort(p.name)
}

// SetConnection sets which connection plugged in to this port.
func (p *defaultPort) SetConnection(conn Connection) {
	if p.conn != nil {
		connName := p.conn.Name()
		newConnName := conn.Name()
		panicMsg := fmt.Sprintf(
			"connection already set to %s, now connecting to %s",
			connName, newConnName,
		)
		panic(panicMsg)
	}

	p.conn = conn
}

// Component returns the owner component of the port.
func (p *defaultPort) Component() Component {
	return p.comp
}

// Name returns the name of the port.
func (p *defaultPort) Name() string {
	return p.name
}

// CanSend checks if the port can send a message without error.
func (p *defaultPort) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	canSend := p.outgoingBuf.CanPush()

	return canSend
}

// Send is used to send a message out from a component
func (p *defaultPort) Send(msg Msg) *SendError {
	p.lock.Lock()

	p.msgMustBeValid(msg)

	if !p.outgoingBuf.CanPush() {
		p.lock.Unlock()
		return NewSendError()
	}

	wasEmpty := (p.outgoingBuf.Size() == 0)
	p.outgoingBuf.Push(msg)

	hookCtx := HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)
	p.lock.Unlock()

	if wasEmpty {
		p.conn.NotifySend()
	}

	return nil
}

// Deliver is used to deliver a message to a component
func (p *defaultPort) Deliver(msg Msg) *SendError {
	p.lock.Lock()

	if !p.incomingBuf.CanPush() {
		p.lock.Unlock()
		return NewSendError()
	}

	wasEmpty := (p.incomingBuf.Size() == 0)

	hookCtx := HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.incomingBuf.Push(msg)
	p.lock.Unlock()

	if p.comp != nil && wasEmpty {
		p.comp.NotifyRecv(p)
	}

	return nil
}

// RetrieveIncoming is used by the component to take a message from the incoming
// buffer
func (p *defaultPort) RetrieveIncoming() Msg {
	p.lock.Lock()

	item := p.incomingBuf.Pop()
	if item == nil {
		p.lock.Unlock()
		return nil
	}

	if p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.conn.NotifyAvailable(p)
	}

	p.lock.Unlock()

	msg := item.(Msg)
	hookCtx := HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveIncoming,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	return msg
}

// RetrieveOutgoing is used by the component to take a message from the outgoing
// buffer
func (p *defaultPort) RetrieveOutgoing() Msg {
	p.lock.Lock()

	item := p.outgoingBuf.Pop()
	if item == nil {
		p.lock.Unlock()
		return nil
	}

	if p.outgoingBuf.Size() == p.outgoingBuf.Capacity()-1 {
		p.comp.NotifyPortFree(p)
	}

	p.lock.Unlock()

	msg := item.(Msg)
	hookCtx := HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveOutgoing,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	return msg
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it.
func (p *defaultPort) PeekIncoming() Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.incomingBuf.Peek()
	if item == nil {
		return nil
	}

	msg := item.(Msg)

	return msg
}

// PeekOutgoing returns the first message in the outgoing buffer without
// removing it.
func (p *defaultPort) PeekOutgoing() Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.outgoingBuf.Peek()
	if item == nil {
		return nil
	}

	msg := item.(Msg)

	return msg
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *defaultPort) NotifyAvailable() {
	if p.comp != nil {
		p.comp.NotifyPortFree(p)
	}
}

// NewPort creates a new port with default behavior.
func NewPort(
	comp Component,
	incomingBufCap, outgoingBufCap int,
	name string,
) Port {
	p := new(defaultPort)
	p.comp = comp
	p.incomingBuf = NewBuffer(name+".IncomingBuf", incomingBufCap)
	p.outgoingBuf = NewBuffer(name+".OutgoingBuf", outgoingBufCap)
	p.name = name

	return p
}

func (p *defaultPort) msgMustBeValid(msg Msg) {
	portMustBeMsgSrc(p, msg)
	dstMustNotBeEmpty(msg.Meta().Dst)
	srcDstMustNotBeTheSame(msg)
}

func portMustBeMsgSrc(port Port, msg Msg) {
	if port.Name() != string(msg.Meta().Src) {
		panic("sending port is not msg src")
	}
}

func dstMustNotBeEmpty(port RemotePort) {
	if port == "" {
		panic("dst is not given")
	}
}

func srcDstMustNotBeTheSame(msg Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic("sending back to src")
	}
}
