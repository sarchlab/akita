// Package directconnection provides directconnection
package directconnection

import (
	"github.com/sarchlab/akita/v4/sim/modeling"
)

type ports struct {
	ports   []modeling.Port
	portMap map[modeling.RemotePort]int
}

func (p *ports) addPort(port modeling.Port) {
	p.ports = append(p.ports, port)
	p.portMap[port.AsRemote()] = len(p.ports) - 1
}

func (p *ports) getPortIndex(index int) modeling.Port {
	return p.ports[index]
}

func (p *ports) getPortByName(name modeling.RemotePort) modeling.Port {
	return p.ports[p.portMap[name]]
}

func (p *ports) list() []modeling.Port {
	return p.ports
}

func (p *ports) len() int {
	return len(p.ports)
}

// Comp is a DirectConnection connects two components without latency
type Comp struct {
	*modeling.TickingComponent
	modeling.MiddlewareHolder

	ports      ports
	nextPortID int
}

// ID returns the name of the component.
func (c *Comp) ID() string {
	return c.Name()
}

// Serialize serializes the component.
func (c *Comp) Serialize() (map[string]any, error) {
	return map[string]any{
		"next_port_id": c.nextPortID,
	}, nil
}

// Deserialize deserializes the component.
func (c *Comp) Deserialize(
	data map[string]any,
) error {
	c.nextPortID = data["next_port_id"].(int)

	return nil
}

// PlugIn marks the port connects to this DirectConnection.
func (c *Comp) PlugIn(port modeling.Port) {
	c.Lock()
	defer c.Unlock()

	c.ports.addPort(port)

	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *Comp) Unplug(_ modeling.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify that the connection can
// deliver to the port again.
func (c *Comp) NotifyAvailable(p modeling.Port) {
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
	port modeling.Port,
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
