// Package directconnection provides directconnection
package directconnection

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
)

type ports struct {
	ports   []sim.Port
	portMap map[sim.RemotePort]int
}

func (p *ports) addPort(port sim.Port) {
	p.ports = append(p.ports, port)
	p.portMap[port.AsRemote()] = len(p.ports) - 1
}

func (p *ports) getPortIndex(index int) sim.Port {
	return p.ports[index]
}

func (p *ports) getPortByName(name sim.RemotePort) sim.Port {
	portIndex, found := p.portMap[name]
	if !found {
		panic(fmt.Sprintf("port %s not found", name))
	}

	return p.ports[portIndex]
}

func (p *ports) list() []sim.Port {
	return p.ports
}

func (p *ports) len() int {
	return len(p.ports)
}

// Comp is a DirectConnection connects two components without latency
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	ports      ports
	nextPortID int
}

// PlugIn marks the port connects to this DirectConnection.
func (c *Comp) PlugIn(port sim.Port) {
	c.Lock()
	defer c.Unlock()

	c.ports.addPort(port)

	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify that the connection can
// deliver to the port again.
func (c *Comp) NotifyAvailable(p sim.Port) {
	for _, port := range c.ports.list() {
		if port == p {
			continue
		}

		port.NotifyAvailable()
	}

	c.TickNow()
}

// NotifySend is called by a port to notify that the connection can start
// to tick now
func (c *Comp) NotifySend() {
	c.TickNow()
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick updates the states of the connection and delivers messages.
func (m *middleware) Tick() bool {
	madeProgress := false

	for i := range m.ports.len() {
		portID := (i + m.nextPortID) % m.ports.len()
		port := m.ports.getPortIndex(portID)
		madeProgress = m.forwardMany(port) || madeProgress
	}

	m.nextPortID = (m.nextPortID + 1) % m.ports.len()

	return madeProgress
}

func (m *middleware) forwardMany(
	port sim.Port,
) bool {
	madeProgress := false

	for {
		head := port.PeekOutgoing()
		if head == nil {
			break
		}

		dst := head.Meta().Dst
		dstPort := m.ports.getPortByName(dst)

		err := dstPort.Deliver(head)
		if err != nil {
			break
		}

		madeProgress = true

		port.RetrieveOutgoing()
	}

	return madeProgress
}
