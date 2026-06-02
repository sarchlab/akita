package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"

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
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

// plugNoopConn plugs a fresh noopConn into every port of the component so the
// component's owned ports can be driven directly in tests.
func plugNoopConn(comp *Comp) {
	conn := &noopConn{}
	conn.PlugIn(comp.GetPortByName("Top"))
	conn.PlugIn(comp.GetPortByName("Bottom"))
	conn.PlugIn(comp.GetPortByName("Control"))
}

// makeDirectConnection builds a direct connection driven by the given engine.
func makeDirectConnection(engine timing.Engine) messaging.Connection {
	return directconnection.MakeBuilder().
		WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
		Build("Conn")
}

// idealEndpoint is a minimal messaging.Component used as the remote peer of the
// TLB in the integration tests. It owns a single real port; when a message is
// delivered to that port it records the message and optionally runs onDeliver.
type idealEndpoint struct {
	hooking.HookableBase
	*messaging.PortOwnerBase

	name          string
	port          messaging.Port
	onDeliver     func(msg messaging.Msg)
	lastDelivered messaging.Msg
}

func newIdealEndpoint(name string) *idealEndpoint {
	ep := &idealEndpoint{
		name:          name,
		PortOwnerBase: messaging.NewPortOwnerBase(),
	}
	ep.port = messaging.NewPort(ep, 4, 4, name+".Port")
	ep.AddPort("Port", ep.port)

	return ep
}

func (ep *idealEndpoint) Name() string { return ep.name }

func (ep *idealEndpoint) NotifyRecv(port messaging.Port) {
	for msg := port.RetrieveIncoming(); msg != nil; msg = port.RetrieveIncoming() {
		ep.lastDelivered = msg
		if ep.onDeliver != nil {
			ep.onDeliver(msg)
		}
	}
}

func (ep *idealEndpoint) NotifyPortFree(_ messaging.Port) {}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
