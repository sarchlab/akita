package wiring

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/v5/comm"
	hooking "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"
)

// Wire connects exactly two wiring ports and enables pull-based transfers.
type Wire struct {
	*hooking.HookableBase

	lock sync.Mutex
	name string

	port1 *Port
	port2 *Port
}

var _ RetrievableConnection = (*Wire)(nil)

// Name implements comm.Named.
func (w *Wire) Name() string {
	return w.name
}

// PlugIn connects a wiring port to the wire.
func (w *Wire) PlugIn(port comm.Port) {
	w.lock.Lock()
	defer w.lock.Unlock()

	wiringPort, ok := port.(*Port)
	if !ok {
		panic("wire can only connect to wiring.Port")
	}

	if w.port1 == nil {
		w.port1 = wiringPort
	} else if w.port2 == nil {
		w.port2 = wiringPort
	} else {
		panic("wire already has two ports connected")
	}

	wiringPort.SetConnection(w)
}

// Unplug removes a port from the wire.
func (w *Wire) Unplug(comm.Port) {
	panic("wiring.Wire.Unplug is not supported")
}

// NotifyAvailable is unused because wiring uses pull-based delivery.
func (w *Wire) NotifyAvailable(comm.Port) {
	panic("wiring.Wire.NotifyAvailable should never be called")
}

// NotifySend notifies both endpoints that the peer has a message ready.
func (w *Wire) NotifySend() {
	w.lock.Lock()
	defer w.lock.Unlock()

	if w.port1 != nil && w.port1.comp != nil {
		w.port1.comp.NotifyRecv(w.port1)
	}
	if w.port2 != nil && w.port2.comp != nil {
		w.port2.comp.NotifyRecv(w.port2)
	}
}

// Peek returns the message currently latched on the opposite port.
func (w *Wire) Peek(port *Port) comm.Msg {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.portMustBeConnected(port)

	if port == w.port1 {
		return w.port2.PeekOutgoing()
	}
	return w.port1.PeekOutgoing()
}

// Retrieve removes and returns the message currently latched on the opposite
// port.
func (w *Wire) Retrieve(port *Port) comm.Msg {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.portMustBeConnected(port)
	otherPort := w.theOtherPort(port)

	msg := otherPort.RetrieveOutgoing()
	if msg != nil {
		srcAndDstMustBeValid(otherPort.AsRemote(), port.AsRemote(), msg)
	}

	return msg
}

// ConnectWithWire instantiates a wire between two wiring ports.
func ConnectWithWire(port1 *Port, port2 *Port) *Wire {
	w := &Wire{
		HookableBase: hooking.NewHookableBase(),
		name:         fmt.Sprintf("%s-%s", port1.Name(), port2.Name()),
	}

	w.PlugIn(port1)
	w.PlugIn(port2)

	return w
}

func (w *Wire) portMustBeConnected(port *Port) {
	if port == nil {
		panic("nil port")
	}

	if port != w.port1 && port != w.port2 {
		panic("port not connected to this wire")
	}
}

func (w *Wire) theOtherPort(port *Port) *Port {
	if port == w.port1 {
		return w.port2
	}
	if port == w.port2 {
		return w.port1
	}

	panic("port not connected to this wire")
}

func srcAndDstMustBeValid(expSrc, expDst comm.RemotePort, msg comm.Msg) {
	if msg.Src() != expSrc || msg.Dst() != expDst {
		panic("message src and dst are not valid for the wire")
	}
}
