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

// LimitNumMsgPort is a type of port that can hold at most a certain number
// of messages.
type LimitNumMsgPort struct {
	HookableBase

	lock sync.Mutex
	name string
	comp Component
	conn Connection

	incomingBuf Buffer
	outgoingBuf Buffer
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

type sampleMsg struct {
	MsgMeta
}

func NewSampleMsg() *sampleMsg {
	m := &sampleMsg{}
	return m
}

func (m *sampleMsg) Meta() *MsgMeta {
	return &m.MsgMeta
}

// Name returns the name of the port.
func (p *LimitNumMsgPort) Name() string {
	return p.name
}

// CanSend checks if the port can send a message without error.
func (p *LimitNumMsgPort) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	canSend := p.outgoingBuf.CanPush()

	return canSend
}

// Send is used to send a message out from a component
func (p *LimitNumMsgPort) Send(msg Msg) *SendError {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.msgMustBeValid(msg)

	if !p.outgoingBuf.CanPush() {
		return NewSendError()
	}

	p.outgoingBuf.Push(msg)
	hookCtx := HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.conn.NotifySend()

	return nil
}

// Deliver is used to deliver a message to a component
func (p *LimitNumMsgPort) Deliver(msg Msg) *SendError {
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.incomingBuf.CanPush() {
		return NewSendError()
	}

	hookCtx := HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.incomingBuf.Push(msg)

	if p.comp != nil {
		p.comp.NotifyRecv(p)
	}

	return nil
}

// RetrieveIncoming is used by the component to take a message from the incoming
// buffer
func (p *LimitNumMsgPort) RetrieveIncoming() Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.incomingBuf.Pop()
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

	if p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.conn.NotifyAvailable(p)
	}

	return msg
}

// RetrieveOutgoing is used by the component to take a message from the outgoing
// buffer
func (p *LimitNumMsgPort) RetrieveOutgoing() Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.outgoingBuf.Pop()
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

	if p.outgoingBuf.Size() == p.outgoingBuf.Capacity()-1 {
		p.comp.NotifyPortFree(p)
	}

	return msg
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it.
func (p *LimitNumMsgPort) PeekIncoming() Msg {
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
func (p *LimitNumMsgPort) PeekOutgoing() Msg {
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
func (p *LimitNumMsgPort) NotifyAvailable() {
	if p.comp != nil {
		p.comp.NotifyPortFree(p)
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
	p.incomingBuf = NewBuffer(name+".IncomingBuf", capacity)
	p.outgoingBuf = NewBuffer(name+".OutgoingBuf", capacity)
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
	p.incomingBuf = buf
	p.name = name
	return p
}

func (p *LimitNumMsgPort) msgMustBeValid(msg Msg) {
	portMustBeMsgSrc(p, msg)
	dstMustNotBeNil(msg.Meta().Dst)
	srcDstMustNotBeTheSame(msg)
}

func portMustBeMsgSrc(port Port, msg Msg) {
	if port != msg.Meta().Src {
		panic("sending port is not msg src")
	}
}

func dstMustNotBeNil(port Port) {
	if port == nil {
		panic("dst is not given")
	}
}

func srcDstMustNotBeTheSame(msg Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic("sending back to src")
	}
}
