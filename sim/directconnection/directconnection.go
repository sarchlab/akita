// Package directconnection provides directconnection
package directconnection

import (
	"github.com/sarchlab/akita/v4/sim"
)

// Comp is a DirectConnection connects two components without latency
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	nextPortID int
	ports      []sim.Port
}

// PlugIn marks the port connects to this DirectConnection.
func (c *Comp) PlugIn(port sim.Port, sourceSideBufSize int) {
	c.Lock()
	defer c.Unlock()

	c.ports = append(c.ports, port)

	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify that the connection can
// deliver to the port again.
func (c *Comp) NotifyAvailable(p sim.Port) {
	for _, port := range c.ports {
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
	for i := 0; i < len(m.ports); i++ {
		portID := (i + m.nextPortID) % len(m.ports)
		port := m.ports[portID]
		madeProgress = m.forwardMany(port) || madeProgress
	}

	m.nextPortID = (m.nextPortID + 1) % len(m.ports)
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

		err := head.Meta().Dst.Deliver(head)
		if err != nil {
			break
		}

		madeProgress = true
		port.RetrieveOutgoing()
	}

	return madeProgress
}
