package port

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/v5/comm"
	hooking "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"
)

var (
	// HookPosPortMsgSend marks when a message leaves the port's outgoing buffer.
	HookPosPortMsgSend = &hooking.HookPos{Name: "Port Msg Send"}

	// HookPosPortMsgRecvd marks when an inbound message arrives at this port.
	HookPosPortMsgRecvd = &hooking.HookPos{Name: "Port Msg Recv"}

	// HookPosPortMsgRetrieveIncoming marks when an inbound message is retrieved
	// from the incoming buffer.
	HookPosPortMsgRetrieveIncoming = &hooking.HookPos{
		Name: "Port Msg Retrieve Incoming",
	}

	// HookPosPortMsgRetrieveOutgoing marks when an outbound message is retrieved
	// from the outgoing buffer.
	HookPosPortMsgRetrieveOutgoing = &hooking.HookPos{
		Name: "Port Msg Retrieve Outgoing",
	}
)

// DefaultPort implements comm.Port with dual FIFO buffers for outgoing and
// incoming traffic. The implementation mirrors the proven v4 behaviour but is
// adapted to the v5 Msg interface.
type DefaultPort struct {
	*hooking.HookableBase

	lock sync.Mutex
	name string
	comp comm.Component
	conn comm.Connection

	incomingBuf *msgBuffer
	outgoingBuf *msgBuffer
}

var _ comm.Port = (*DefaultPort)(nil)

// AsRemote returns the remote port name.
func (p *DefaultPort) AsRemote() comm.RemotePort {
	return comm.RemotePort(p.name)
}

// SetConnection records the connection that is plugged into this port.
func (p *DefaultPort) SetConnection(conn comm.Connection) {
	p.lock.Lock()
	defer p.lock.Unlock()

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
func (p *DefaultPort) Component() comm.Component {
	return p.comp
}

// Name returns the name of the port.
func (p *DefaultPort) Name() string {
	return p.name
}

// CanSend checks if the port can send a message without error.
func (p *DefaultPort) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.CanPush()
}

// Send enqueues a message so the connection can deliver it to the destination.
func (p *DefaultPort) Send(msg comm.Msg) *comm.SendError {
	p.lock.Lock()

	p.msgMustBeValid(msg)

	if !p.outgoingBuf.CanPush() {
		p.lock.Unlock()
		return comm.NewSendError()
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

	if wasEmpty && p.conn != nil {
		p.conn.NotifySend()
	}

	return nil
}

// Deliver is used by a connection to deliver a message to this port.
func (p *DefaultPort) Deliver(msg comm.Msg) *comm.SendError {
	p.lock.Lock()

	if !p.incomingBuf.CanPush() {
		p.lock.Unlock()
		return comm.NewSendError()
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

	return nil
}

// RetrieveIncoming is used by the component to take a message from the incoming
// buffer.
func (p *DefaultPort) RetrieveIncoming() comm.Msg {
	p.lock.Lock()

	msg := p.incomingBuf.Pop()
	if msg == nil {
		p.lock.Unlock()
		return nil
	}

	if p.conn != nil && p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.conn.NotifyAvailable(p)
	}

	p.lock.Unlock()

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveIncoming,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	return msg
}

// RetrieveOutgoing is used by the component to take a message from the outgoing
// buffer.
func (p *DefaultPort) RetrieveOutgoing() comm.Msg {
	p.lock.Lock()

	msg := p.outgoingBuf.Pop()
	if msg == nil {
		p.lock.Unlock()
		return nil
	}

	notify := p.comp != nil && p.outgoingBuf.Size() == p.outgoingBuf.Capacity()-1

	p.lock.Unlock()

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveOutgoing,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if notify {
		p.comp.NotifyPortFree(p)
	}

	return msg
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it.
func (p *DefaultPort) PeekIncoming() comm.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.Peek()
}

// PeekOutgoing returns the first message in the outgoing buffer without
// removing it.
func (p *DefaultPort) PeekOutgoing() comm.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.Peek()
}

// NotifyAvailable is called by the connection to notify the port that the
// connection has capacity again.
func (p *DefaultPort) NotifyAvailable() {
	if p.comp != nil {
		p.comp.NotifyPortFree(p)
	}
}

// NewPort creates a new port with default buffering behaviour.
func NewPort(
	comp comm.Component,
	incomingBufCap, outgoingBufCap int,
	name string,
) *DefaultPort {
	p := &DefaultPort{
		HookableBase: hooking.NewHookableBase(),
		comp:         comp,
		incomingBuf:  newMsgBuffer(name+".IncomingBuf", incomingBufCap),
		outgoingBuf:  newMsgBuffer(name+".OutgoingBuf", outgoingBufCap),
		name:         name,
	}

	return p
}

func (p *DefaultPort) msgMustBeValid(msg comm.Msg) {
	if msg == nil {
		panic("sending nil msg")
	}
	portMustBeMsgSrc(p, msg)
	dstMustNotBeEmpty(msg.Dst())
	srcDstMustNotBeTheSame(msg)
}

func portMustBeMsgSrc(port *DefaultPort, msg comm.Msg) {
	if port.Name() != string(msg.Src()) {
		panic("sending port is not msg src")
	}
}

func dstMustNotBeEmpty(port comm.RemotePort) {
	if port == "" {
		panic("dst is not given")
	}
}

func srcDstMustNotBeTheSame(msg comm.Msg) {
	if msg.Src() == msg.Dst() {
		panic("sending back to src")
	}
}

// msgBuffer is a bounded FIFO queue specialised for comm.Msg values.
type msgBuffer struct {
	name     string
	capacity int
	items    []comm.Msg
}

func newMsgBuffer(name string, capacity int) *msgBuffer {
	if capacity < 0 {
		panic("buffer capacity must be non-negative")
	}
	return &msgBuffer{
		name:     name,
		capacity: capacity,
		items:    make([]comm.Msg, 0, capacity),
	}
}

func (b *msgBuffer) CanPush() bool {
	return len(b.items) < b.capacity
}

func (b *msgBuffer) Push(msg comm.Msg) {
	if !b.CanPush() {
		panic("buffer overflow")
	}
	b.items = append(b.items, msg)
}

func (b *msgBuffer) Pop() comm.Msg {
	if len(b.items) == 0 {
		return nil
	}

	msg := b.items[0]
	b.items[0] = nil
	b.items = b.items[1:]

	return msg
}

func (b *msgBuffer) Peek() comm.Msg {
	if len(b.items) == 0 {
		return nil
	}

	return b.items[0]
}

func (b *msgBuffer) Capacity() int {
	return b.capacity
}

func (b *msgBuffer) Size() int {
	return len(b.items)
}
