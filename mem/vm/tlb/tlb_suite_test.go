package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTlb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tlb Suite")
}

// noopConn is a minimal messaging.Connection used to drive a component's real
// ports in isolation. Because the TLB now owns its ports (they are no longer
// injectable), tests feed requests with Deliver and read responses with
// RetrieveOutgoing; the port still needs a connection so its send/retrieve
// notifications have somewhere to go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

// plugNoopConn plugs a fresh noopConn into every port of the component so the
// component's owned ports can be driven directly in tests.
func plugNoopConn(comp *Comp) {
	conn := &noopConn{}
	conn.BindPort(comp.Ports.Top)
	conn.BindPort(comp.Ports.Bottom)
	conn.BindPort(comp.Ports.Control)
}

// defaultPorts creates the ports of the TLB named name, with the historical
// default buffer sizes.
func defaultPorts(name string) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(4, 4),
		Bottom:  twowaybuffered.NewPort(4, 4),
		Control: twowaybuffered.NewPort(1, 1),
	}
}

// makeDirectConnection builds a direct connection using the given simulation.
func makeDirectConnection(sim timing.Simulation) messaging.Connection {
	return direct.NewConnection("Conn", sim, timing.GHz)
}

// idealEndpoint is a minimal messaging.PortOwner used as the remote peer of the
// TLB in the integration tests. It owns a single real port; when a message is
// delivered to that port it records the message and optionally runs onDeliver.
type idealEndpoint struct {
	hooking.HookableBase

	name          string
	port          messaging.Port
	onDeliver     func(msg messaging.Msg)
	lastDelivered messaging.Msg
}

func newIdealEndpoint(name string) *idealEndpoint {
	ep := &idealEndpoint{
		name: name,
	}
	ep.port = twowaybuffered.NewPort(4, 4)
	ep.port.BindOwner(ep, name+".Port")

	return ep
}

func (ep *idealEndpoint) Name() string { return ep.name }

func (ep *idealEndpoint) NotifyRecv(port messaging.Port) {
	for msg, ok := port.RetrieveIncoming(); ok; msg, ok = port.RetrieveIncoming() {
		ep.lastDelivered = msg
		if ep.onDeliver != nil {
			ep.onDeliver(msg)
		}
	}
}

func (ep *idealEndpoint) NotifyPortFree(_ messaging.Port) {}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
