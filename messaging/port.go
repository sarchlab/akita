package messaging

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/queueing"
)

// HookPosPortMsgSend marks when a message is sent out from the port.
var HookPosPortMsgSend = &hooking.HookPos{Name: "Port Msg Send"}

// HookPosPortMsgRecvd marks when an inbound message arrives at a the given port.
var HookPosPortMsgRecvd = &hooking.HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieveIncoming marks when an inbound message is retrieved
// from the incoming buffer.
var HookPosPortMsgRetrieveIncoming = &hooking.HookPos{
	Name: "Port Msg Retrieve Incoming",
}

// HookPosPortMsgRetrieveOutgoing marks when an outbound message is retrieved
// from the outgoing buffer.
var HookPosPortMsgRetrieveOutgoing = &hooking.HookPos{
	Name: "Port Msg Retrieve Outgoing",
}

// A RemotePort is a string that refers to another port.
type RemotePort string

// A Port is owned by a component and is used to plug in connections.
// Peek and Retrieve return nil, false when the corresponding buffer is empty.
// Retrieve removes a message and notifies its sender when a full buffer gains
// space. Capacity and peek checks do not reserve space or messages.
type Port interface {
	naming.Named
	hooking.Hookable

	AsRemote() RemotePort

	SetConnection(conn Connection)
	Component() Component
	SetComponent(comp Component)

	// For connection
	CanDeliver() bool
	Deliver(msg Msg)
	NotifyAvailable()
	RetrieveOutgoing() (Msg, bool)
	PeekOutgoing() (Msg, bool)

	// For component
	CanSend() bool
	Send(msg Msg)
	RetrieveIncoming() (Msg, bool)
	PeekIncoming() (Msg, bool)

	// Buffer counts
	NumIncoming() int
	NumOutgoing() int
}

// DefaultPort implements the Port interface.
type defaultPort struct {
	hooking.HookableBase

	lock sync.Mutex
	name string
	comp Component
	conn Connection

	incomingBuf queueing.Buffer[Msg]
	outgoingBuf queueing.Buffer[Msg]
}

// AsRemote returns the remote port name.
func (p *defaultPort) AsRemote() RemotePort {
	return RemotePort(p.name)
}

// SetConnection sets which connection is plugged in to this port.
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

// Connection returns the connection plugged in to this port, or nil if the port
// is not connected. It is the read-side counterpart of SetConnection and lets
// the topology recorder reconstruct the component graph from the port side.
func (p *defaultPort) Connection() Connection {
	return p.conn
}

// Component returns the owner component of the port.
func (p *defaultPort) Component() Component {
	return p.comp
}

// SetComponent sets the owner component of the port.
func (p *defaultPort) SetComponent(comp Component) {
	p.comp = comp
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

// Send is used to send a message out from a component. The caller must verify
// the port has capacity with CanSend before calling Send; sending into a full
// outgoing buffer is a programming error and will panic.
func (p *defaultPort) Send(msg Msg) {
	p.lock.Lock()

	p.msgMustBeValid(msg)

	if !p.outgoingBuf.CanPush() {
		p.lock.Unlock()
		panic(fmt.Sprintf(
			"Send called on port %s with full outgoing buffer; "+
				"caller must check CanSend first",
			p.name,
		))
	}

	wasEmpty := (p.outgoingBuf.Size() == 0)
	p.outgoingBuf.Push(msg)

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)
	p.lock.Unlock()

	if wasEmpty {
		p.conn.NotifySend()
	}
}

// CanDeliver checks if the port can accept an incoming message without error.
func (p *defaultPort) CanDeliver() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.CanPush()
}

// Deliver is used to deliver a message to a component. The caller must verify
// the port has capacity with CanDeliver before calling Deliver; delivering
// into a full incoming buffer is a programming error and will panic.
func (p *defaultPort) Deliver(msg Msg) {
	p.lock.Lock()

	if !p.incomingBuf.CanPush() {
		p.lock.Unlock()
		panic(fmt.Sprintf(
			"Deliver called on port %s with full incoming buffer; "+
				"caller must check CanDeliver first",
			p.name,
		))
	}

	wasEmpty := (p.incomingBuf.Size() == 0)

	hookCtx := hooking.HookCtx{
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
}

// RetrieveIncoming is used by the component to take a message from the
// incoming buffer. The boolean reports whether a message was present.
func (p *defaultPort) RetrieveIncoming() (Msg, bool) {
	p.lock.Lock()

	msg, ok := p.incomingBuf.Pop()
	if !ok {
		p.lock.Unlock()
		return nil, false
	}

	if p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.conn.NotifyAvailable(p)
	}

	p.lock.Unlock()

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveIncoming,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	return msg, true
}

// RetrieveOutgoing is used by the connection to take a message from the outgoing
// buffer. The boolean reports whether a message was present.
func (p *defaultPort) RetrieveOutgoing() (Msg, bool) {
	p.lock.Lock()

	msg, ok := p.outgoingBuf.Pop()
	if !ok {
		p.lock.Unlock()
		return nil, false
	}

	if p.outgoingBuf.Size() == p.outgoingBuf.Capacity()-1 {
		p.comp.NotifyPortFree(p)
	}

	p.lock.Unlock()

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveOutgoing,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	return msg, true
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it. The boolean reports whether a message was present.
func (p *defaultPort) PeekIncoming() (Msg, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.Peek()
}

// PeekOutgoing returns the first message in the outgoing buffer without
// removing it. The boolean reports whether a message was present.
func (p *defaultPort) PeekOutgoing() (Msg, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.Peek()
}

// NumIncoming returns the number of messages in the incoming buffer.
func (p *defaultPort) NumIncoming() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.Size()
}

// NumOutgoing returns the number of messages in the outgoing buffer.
func (p *defaultPort) NumOutgoing() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.Size()
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again.
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
	p.incomingBuf = queueing.NewBuffer[Msg](name+".Incoming", incomingBufCap)
	p.outgoingBuf = queueing.NewBuffer[Msg](name+".Outgoing", outgoingBufCap)
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
