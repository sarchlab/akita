package wiring

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/sim"
)

// RetrievableConnection is a connection that can retrieve and peek messages
// that are destined to this port.
type RetrievableConnection interface {
	sim.Connection

	Retrieve(dst *Port) sim.Msg
	Peek(dst *Port) sim.Msg
}

// A Port provided by the wiring package provides only a single slot in the
// outgoing buffer. There is no incoming buffer.
type Port struct {
	sim.HookableBase

	lock       sync.Mutex
	name       string
	comp       sim.Component
	conn       RetrievableConnection
	timeTeller sim.TimeTeller

	msgToSend sim.Msg
	sendTime  sim.VTimeInSec
}

// AsRemote returns the remote port name.
func (p *Port) AsRemote() sim.RemotePort {
	return sim.RemotePort(p.name)
}

// SetConnection sets which connection plugged in to this port.
func (p *Port) SetConnection(conn sim.Connection) {
	p.lock.Lock()
	defer p.lock.Unlock()

	rConn, ok := conn.(RetrievableConnection)
	if !ok {
		panic("connection must implement RetrievableConnection")
	}

	if p.conn != nil {
		connName := p.conn.Name()
		newConnName := conn.Name()
		panicMsg := fmt.Sprintf(
			"connection already set to %s, now connecting to %s",
			connName, newConnName,
		)
		panic(panicMsg)
	}

	p.conn = rConn
}

// Component returns the owner component of the port.
func (p *Port) Component() sim.Component {
	return p.comp
}

// Name returns the name of the port.
func (p *Port) Name() string {
	return p.name
}

// CanSend checks if the port can send a message without error.
func (p *Port) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.msgToSend == nil
}

// Send is used to send a message out from a component
func (p *Port) Send(msg sim.Msg) *sim.SendError {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.msgMustBeValid(msg)

	if p.msgToSend != nil {
		return sim.NewSendError()
	}

	p.msgToSend = msg
	p.sendTime = p.timeTeller.CurrentTime()

	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    sim.HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.conn.NotifySend()

	return nil
}

func (p *Port) msgMustBeValid(msg sim.Msg) {
	if msg.Meta().Src == "" || msg.Meta().Dst == "" {
		panic("message src and dst must be set")
	}

	if msg.Meta().Src != p.AsRemote() {
		panic("message src must be the port")
	}
}

// Deliver is used to deliver a message to a component
func (p *Port) Deliver(msg sim.Msg) *sim.SendError {
	// This port does not accept connection-delivered incoming messages.
	// Use PeekIncoming and PeekOutgoing instead.
	return sim.NewSendError()
}

// RetrieveIncoming is used by the component to take a message from the incoming
// buffer
func (p *Port) RetrieveIncoming() sim.Msg {
	return p.conn.Retrieve(p)
}

// RetrieveOutgoing is used by the component to take a message from the outgoing
// buffer
func (p *Port) RetrieveOutgoing() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	msg := p.msgToSend
	if msg == nil {
		return nil
	}

	// Don't allow retrieval in the same cycle
	if p.sendTime == p.timeTeller.CurrentTime() {
		return nil
	}

	p.msgToSend = nil

	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    sim.HookPosPortMsgRetrieveOutgoing,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if p.comp != nil {
		p.comp.NotifyPortFree(p)
	}

	return msg
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it.
func (p *Port) PeekIncoming() sim.Msg {
	return p.conn.Peek(p)
}

// PeekOutgoing returns the first message in the outgoing buffer without
// removing it.
func (p *Port) PeekOutgoing() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.msgToSend == nil {
		return nil
	}

	// Don't allow peeking in the same cycle
	if p.sendTime == p.timeTeller.CurrentTime() {
		return nil
	}

	return p.msgToSend
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *Port) NotifyAvailable() {
	panic("port's NotifyAvailable should never be called")
}

// NewPort creates a new port with the simplified wiring implementation.
func NewPort(comp sim.Component, name string, timeTeller sim.TimeTeller) *Port {
	p := new(Port)
	p.comp = comp
	p.name = name
	p.timeTeller = timeTeller

	return p
}
