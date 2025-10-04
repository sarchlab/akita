package wiring

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/v5/comm"
	hooking "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"
	"github.com/sarchlab/akita/v4/v5/timing"
)

// RetrievableConnection provides random-access helpers so wiring ports can
// directly inspect and drain their counterpart's outgoing slot.
type RetrievableConnection interface {
	comm.Connection

	Retrieve(dst *Port) comm.Msg
	Peek(dst *Port) comm.Msg
}

var (
	// HookPosPortMsgSend marks when a wiring port latches a new outgoing msg.
	HookPosPortMsgSend = &hooking.HookPos{Name: "Port Msg Send"}

	// HookPosPortMsgRetrieveOutgoing marks when the component reclaims its
	// latched message for transmission.
	HookPosPortMsgRetrieveOutgoing = &hooking.HookPos{
		Name: "Port Msg Retrieve Outgoing",
	}
)

// Port provides a single-slot handoff optimised for tightly-coupled blocks.
//
// Unlike the default port implementation, this variant does not maintain an
// inbound FIFO. Instead, receivers actively pull data from the remote sender via
// the shared wiring connection.
type Port struct {
	*hooking.HookableBase

	lock       sync.Mutex
	name       string
	comp       comm.Component
	conn       RetrievableConnection
	timeTeller comm.TimeTeller

	msgToSend comm.Msg
	sendTime  timing.VTimeInSec
}

var _ comm.Port = (*Port)(nil)

// AsRemote returns the remote identifier for this port.
func (p *Port) AsRemote() comm.RemotePort {
	return comm.RemotePort(p.name)
}

// SetConnection registers the wiring connection.
func (p *Port) SetConnection(conn comm.Connection) {
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

// Component returns the component that owns the port.
func (p *Port) Component() comm.Component {
	return p.comp
}

// Name returns the name of the port.
func (p *Port) Name() string {
	return p.name
}

// CanSend returns true if the single outgoing slot is empty.
func (p *Port) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.msgToSend == nil
}

// Send latches a message into the port for the remote side to retrieve.
func (p *Port) Send(msg comm.Msg) *comm.SendError {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.msgMustBeValid(msg)

	if p.msgToSend != nil {
		return comm.NewSendError()
	}

	p.msgToSend = msg
	if p.timeTeller != nil {
		p.sendTime = p.timeTeller.CurrentTime()
	}

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if p.conn != nil {
		p.conn.NotifySend()
	}

	return nil
}

func (p *Port) msgMustBeValid(msg comm.Msg) {
	if msg == nil {
		panic("message must not be nil")
	}
	if msg.Src() == "" || msg.Dst() == "" {
		panic("message src and dst must be set")
	}

	if msg.Src() != p.AsRemote() {
		panic("message src must be the port")
	}
}

// Deliver is not supported because wiring ports rely on pull semantics.
func (p *Port) Deliver(comm.Msg) *comm.SendError {
	return comm.NewSendError()
}

// RetrieveIncoming pulls the next message sourced from the remote peer.
func (p *Port) RetrieveIncoming() comm.Msg {
	if p.conn == nil {
		return nil
	}
	return p.conn.Retrieve(p)
}

// RetrieveOutgoing allows the component to reclaim the latched message once the
// wire drained it.
func (p *Port) RetrieveOutgoing() comm.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	msg := p.msgToSend
	if msg == nil {
		return nil
	}

	if p.timeTeller != nil && p.sendTime == p.timeTeller.CurrentTime() {
		return nil
	}

	p.msgToSend = nil

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveOutgoing,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if p.comp != nil {
		p.comp.NotifyPortFree(p)
	}

	return msg
}

// PeekIncoming inspects the remote sender's latched message without consuming
// it.
func (p *Port) PeekIncoming() comm.Msg {
	if p.conn == nil {
		return nil
	}
	return p.conn.Peek(p)
}

// PeekOutgoing returns the currently latched message, unless it was just sent
// in this time step.
func (p *Port) PeekOutgoing() comm.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.msgToSend == nil {
		return nil
	}

	if p.timeTeller != nil && p.sendTime == p.timeTeller.CurrentTime() {
		return nil
	}

	return p.msgToSend
}

// NotifyAvailable is unused because the counterpart pulls directly via Retrieve.
func (p *Port) NotifyAvailable() {
	panic("wiring.Port.NotifyAvailable should never be called")
}

// NewPort creates a wiring port bound to the provided component.
func NewPort(comp comm.Component, name string, timeTeller comm.TimeTeller) *Port {
	return &Port{
		HookableBase: hooking.NewHookableBase(),
		comp:         comp,
		name:         name,
		timeTeller:   timeTeller,
	}
}
