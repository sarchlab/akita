package wiring

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/sim"
)

// A Wire is a connection between two ports.
//
// The wire implementation can only connect ports defined in this package. The
// uniqueness of wire is it has an extra `Peek` and `Retrieve` method. So, when
// the port calls `Peek` or `Retrieve`, the wire will return the message that is
// buffered in the outgoing buffer on the other side of the wire.
type Wire struct {
	sim.HookableBase

	lock sync.Mutex
	name string

	port1 *Port
	port2 *Port
}

// Name returns the name of the wire.
func (w *Wire) Name() string {
	return w.name
}

// PlugIn connects a port to the wire.
func (w *Wire) PlugIn(port sim.Port) {
	w.lock.Lock()
	defer w.lock.Unlock()

	// Convert to wiring.Port
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
func (w *Wire) Unplug(port sim.Port) {
	panic("unplug is not supported for wiring.Wire")
}

// NotifyAvailable notifies the wire that a port is available to receive
// messages.
func (w *Wire) NotifyAvailable(port sim.Port) {
	panic("wire's NotifyAvailable should never be called")
}

// NotifySend notifies the wire that a port has a message to send.
func (w *Wire) NotifySend() {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.port1.comp.NotifyRecv(w.port1)
	w.port2.comp.NotifyRecv(w.port2)
}

// peek returns the message that is currently in the outgoing buffer of the
// other port.
func (w *Wire) Peek(port *Port) sim.Msg {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.portMustBeConnected(port)

	if port == w.port1 {
		return w.port2.PeekOutgoing()
	} else if port == w.port2 {
		return w.port1.PeekOutgoing()
	}

	panic("port not connected to this wire")
}

// retrieve returns and removes the message that is currently in the outgoing
// buffer of the other port.
func (w *Wire) Retrieve(port *Port) sim.Msg {
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

func ConnectWithWire(port1 *Port, port2 *Port) *Wire {
	w := new(Wire)

	w.name = fmt.Sprintf("%s-%s", port1.Name(), port2.Name())

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
	} else if port == w.port2 {
		return w.port1
	}

	panic("port not connected to this wire")
}

func srcAndDstMustBeValid(
	expSrc, expDst sim.RemotePort,
	msg sim.Msg,
) {
	if msg.Meta().Src != expSrc || msg.Meta().Dst != expDst {
		panic("message src and dst is not valid for the wire")
	}
}
