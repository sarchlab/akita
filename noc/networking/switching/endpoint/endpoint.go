package endpoint

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// Comp is an akita component(Endpoint) that delegates sending and receiving
// actions of a few ports.
type Comp struct {
	*modeling.Component[Spec, State]
}

// outgoingMW returns the outgoing middleware from the component's middleware list.
func (c *Comp) outgoingMW() *outgoingMW {
	return c.Middlewares()[0].(*outgoingMW)
}

// incomingMW returns the incoming middleware from the component's middleware list.
func (c *Comp) incomingMW() *incomingMW {
	return c.Middlewares()[1].(*incomingMW)
}

// NetworkPort returns the network port of the endpoint.
func (c *Comp) NetworkPort() sim.Port {
	return c.outgoingMW().networkPort
}

// SetNetworkPort sets the network port of the endpoint.
func (c *Comp) SetNetworkPort(p sim.Port) {
	c.outgoingMW().networkPort = p
	c.incomingMW().networkPort = p
}

// DefaultSwitchDst returns the default switch destination.
func (c *Comp) DefaultSwitchDst() sim.RemotePort {
	return c.outgoingMW().defaultSwitchDst
}

// SetDefaultSwitchDst sets the default switch destination.
func (c *Comp) SetDefaultSwitchDst(dst sim.RemotePort) {
	c.outgoingMW().defaultSwitchDst = dst
}

// PlugIn connects a port to the endpoint.
func (c *Comp) PlugIn(port sim.Port) {
	port.SetConnection(c)
	c.outgoingMW().devicePorts = append(c.outgoingMW().devicePorts, port)
	c.incomingMW().devicePorts = append(c.incomingMW().devicePorts, port)
}

// NotifyAvailable triggers the endpoint to continue to tick.
func (c *Comp) NotifyAvailable(_ sim.Port) {
	c.TickLater()
}

// NotifySend is called by a port to notify the connection there are
// messages waiting to be sent, can start tick
func (c *Comp) NotifySend() {
	c.TickLater()
}

// Unplug removes the association of a port and an endpoint.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}
