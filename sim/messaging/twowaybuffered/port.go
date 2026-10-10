// Package twowaybuffered provides ports with independent incoming and outgoing buffers.
package twowaybuffered

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/queueing"
)

// Port implements messaging.Port with independently sized incoming and outgoing buffers.
type Port struct {
	hooking.HookableBase

	lock  sync.Mutex
	name  string
	owner messaging.PortOwner
	conn  messaging.Connection

	incomingBuf queueing.Buffer[messaging.Msg]
	outgoingBuf queueing.Buffer[messaging.Msg]
}

// AsRemote returns the remote port name.
func (p *Port) AsRemote() messaging.RemotePort {
	if p.owner == nil {
		panic("twowaybuffered: port must have an owner before AsRemote")
	}
	return messaging.RemotePort(p.name)
}

// BindConnection sets the reverse attachment once, during setup.
// Assemblers normally call Connection.BindPort, which updates both sides.
func (p *Port) BindConnection(conn messaging.Connection) {
	if conn == nil || (reflect.ValueOf(conn).Kind() == reflect.Pointer && reflect.ValueOf(conn).IsNil()) {
		panic("twowaybuffered: cannot bind a nil connection")
	}
	if p.owner == nil {
		panic("twowaybuffered: bind owner before connection")
	}
	if owner, ok := p.owner.(interface{ RequireSetup() }); ok {
		owner.RequireSetup()
	}
	if c, ok := conn.(interface{ RequireSetup() }); ok {
		c.RequireSetup()
	}
	if p.conn != nil {
		panic("twowaybuffered: port already has a connection")
	}

	p.conn = conn
}

// Connection returns the connection plugged in to this port, or nil if the port
// is not connected. It is the read-side counterpart of BindConnection and lets
// the topology recorder reconstruct the component graph from the port side.
func (p *Port) Connection() messaging.Connection {
	return p.conn
}

// Owner returns the owner of the port, or nil if it has none yet.
func (p *Port) Owner() messaging.PortOwner {
	return p.owner
}

// BindOwner assigns the owner and name once. Component.BindPort coordinates
// the declared slot and registration; custom owners call this method directly.
func (p *Port) BindOwner(owner messaging.PortOwner, name string) {
	if owner == nil || (reflect.ValueOf(owner).Kind() == reflect.Pointer && reflect.ValueOf(owner).IsNil()) {
		panic("twowaybuffered: cannot bind a nil owner")
	}
	if p.owner != nil {
		panic("twowaybuffered: port already has an owner")
	}
	naming.MustBeValid(name)
	if c, ok := owner.(interface{ RequireSetup() }); ok {
		c.RequireSetup()
	}
	p.owner = owner
	p.name = name
}

// Name returns the name of the port.
func (p *Port) Name() string {
	return p.name
}

// CanSend checks if the port can send a message without error.
func (p *Port) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	canSend := p.outgoingBuf.CanPush()

	return canSend
}

// Send is used to send a message out from a component. The caller must verify
// the port has capacity with CanSend before calling Send; sending into a full
// outgoing buffer is a programming error and will panic.
func (p *Port) Send(msg messaging.Msg) {
	p.msgMustBeValid(msg)

	p.lock.Lock()

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

	if p.NumHooks() > 0 {
		hookCtx := hooking.HookCtx{
			Domain: p,
			Pos:    messaging.HookPosPortMsgSend,
			Item:   msg,
		}
		p.InvokeHook(hookCtx)
	}
	p.lock.Unlock()

	if wasEmpty {
		p.conn.NotifySend()
	}
}

// CanDeliver checks if the port can accept an incoming message without error.
func (p *Port) CanDeliver() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.CanPush()
}

// Deliver is used to deliver a message to a component. The caller must verify
// the port has capacity with CanDeliver before calling Deliver; delivering
// into a full incoming buffer is a programming error and will panic.
func (p *Port) Deliver(msg messaging.Msg) {
	owner := p.mustHaveOwner()

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

	if p.NumHooks() > 0 {
		hookCtx := hooking.HookCtx{
			Domain: p,
			Pos:    messaging.HookPosPortMsgRecvd,
			Item:   msg,
		}
		p.InvokeHook(hookCtx)
	}

	p.incomingBuf.Push(msg)
	p.lock.Unlock()

	if wasEmpty {
		owner.NotifyRecv(p)
	}
}

// RetrieveIncoming is used by the component to take a message from the
// incoming buffer. The boolean reports whether a message was present.
func (p *Port) RetrieveIncoming() (messaging.Msg, bool) {
	p.lock.Lock()

	msg, ok := p.incomingBuf.Pop()
	if !ok {
		p.lock.Unlock()
		return messaging.Msg{}, false
	}

	if p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.conn.NotifyAvailable(p)
	}

	p.lock.Unlock()

	if p.NumHooks() > 0 {
		hookCtx := hooking.HookCtx{
			Domain: p,
			Pos:    messaging.HookPosPortMsgRetrieveIncoming,
			Item:   msg,
		}
		p.InvokeHook(hookCtx)
	}

	return msg, true
}

// RetrieveOutgoing is used by the connection to take a message from the outgoing
// buffer. The boolean reports whether a message was present.
func (p *Port) RetrieveOutgoing() (messaging.Msg, bool) {
	owner := p.mustHaveOwner()

	p.lock.Lock()

	msg, ok := p.outgoingBuf.Pop()
	if !ok {
		p.lock.Unlock()
		return messaging.Msg{}, false
	}

	if p.outgoingBuf.Size() == p.outgoingBuf.Capacity()-1 {
		owner.NotifyPortFree(p)
	}

	p.lock.Unlock()

	if p.NumHooks() > 0 {
		hookCtx := hooking.HookCtx{
			Domain: p,
			Pos:    messaging.HookPosPortMsgRetrieveOutgoing,
			Item:   msg,
		}
		p.InvokeHook(hookCtx)
	}

	return msg, true
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it. The boolean reports whether a message was present.
func (p *Port) PeekIncoming() (messaging.Msg, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.Peek()
}

// PeekOutgoing returns the first message in the outgoing buffer without
// removing it. The boolean reports whether a message was present.
func (p *Port) PeekOutgoing() (messaging.Msg, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.Peek()
}

// NumIncoming returns the number of messages in the incoming buffer.
func (p *Port) NumIncoming() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.Size()
}

// NumOutgoing returns the number of messages in the outgoing buffer.
func (p *Port) NumOutgoing() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.Size()
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again.
func (p *Port) NotifyAvailable() {
	p.mustHaveOwner().NotifyPortFree(p)
}

// mustHaveOwner returns the port's owner. A port carries traffic only after
// it is bound: a component's BindPort binds its ports, and an owner written
// without a component model calls BindOwner. A port without an owner is a wiring
// error, so it panics.
func (p *Port) mustHaveOwner() messaging.PortOwner {
	if p.owner == nil {
		panic(fmt.Sprintf("twowaybuffered: port %q has no owner; a component's "+
			"BindPort binds its ports, and any other owner must call BindOwner", p.name))
	}

	return p.owner
}

// NewPort creates a port with default behavior, an incoming buffer, and an
// outgoing buffer. The port has no owner yet: a component's BindPort binds it to
// the component, and an owner written without a component model calls
// BindOwner.
func NewPort(incomingBufCap, outgoingBufCap int) *Port {
	p := new(Port)
	p.incomingBuf = queueing.MakeBuffer[messaging.Msg](incomingBufCap)
	p.outgoingBuf = queueing.MakeBuffer[messaging.Msg](outgoingBufCap)
	return p
}

func (p *Port) msgMustBeValid(msg messaging.Msg) {
	if !messaging.PayloadRegistered(msg.Payload) {
		panic(fmt.Sprintf("messaging: unregistered payload type %T; register a value through DefineProtocol", msg.Payload))
	}
	portMustBeMsgSrc(p, msg)
	dstMustNotBeEmpty(msg.Dst)
	srcDstMustNotBeTheSame(msg)
}

func portMustBeMsgSrc(port messaging.Port, msg messaging.Msg) {
	if port.Name() != string(msg.Src) {
		panic("sending port is not msg src")
	}
}

func dstMustNotBeEmpty(port messaging.RemotePort) {
	if port == "" {
		panic("dst is not given")
	}
}

func srcDstMustNotBeTheSame(msg messaging.Msg) {
	if msg.Src == msg.Dst {
		panic("sending back to src")
	}
}
