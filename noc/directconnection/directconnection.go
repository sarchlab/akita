// Package directconnection provides directconnection
package directconnection

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
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

// Comp is a DirectConnection that connects components without latency.
type Comp struct {
	*modeling.Component[Spec, State]
}

func (c *Comp) mw() *middleware {
	return c.Middlewares()[0].(*middleware)
}

// PlugIn marks the port connects to this DirectConnection.
func (c *Comp) PlugIn(port sim.Port) {
	c.Lock()
	defer c.Unlock()

	c.mw().ports.addPort(port)
	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify the connection can deliver again.
func (c *Comp) NotifyAvailable(p sim.Port) {
	for _, port := range c.mw().ports.list() {
		if port == p {
			continue
		}
		port.NotifyAvailable()
	}
	c.TickNow()
}

// NotifySend is called by a port to notify the connection can start ticking.
func (c *Comp) NotifySend() {
	c.TickNow()
}

type middleware struct {
	comp    *modeling.Component[Spec, State]
	ports   ports
	stalled map[int][]sim.Msg // per-port index → stalled messages
}

func (m *middleware) Tick() bool {
	state := m.comp.GetState()
	numPorts := m.ports.len()
	madeProgress := false

	for i := range numPorts {
		portID := (i + state.NextPortID) % numPorts
		port := m.ports.getPortIndex(portID)
		madeProgress = m.forwardMany(port, portID) || madeProgress
	}

	m.comp.GetNextState().NextPortID = (state.NextPortID + 1) % numPorts

	return madeProgress
}

func (m *middleware) forwardMany(port sim.Port, portIdx int) bool {
	madeProgress := false

	// First, try to deliver previously stalled messages for this port.
	if stalledMsgs := m.stalled[portIdx]; len(stalledMsgs) > 0 {
		remaining := stalledMsgs[:0] // reuse slice
		for _, msg := range stalledMsgs {
			dst := msg.Meta().Dst
			dstPort := m.ports.getPortByName(dst)
			err := dstPort.Deliver(msg)
			if err != nil {
				remaining = append(remaining, msg)
			} else {
				madeProgress = true
			}
		}
		m.stalled[portIdx] = remaining
	}

	// Then, drain the outgoing buffer, delivering or stalling each message.
	for {
		head := port.PeekOutgoing()
		if head == nil {
			break
		}
		dst := head.Meta().Dst
		dstPort := m.ports.getPortByName(dst)
		err := dstPort.Deliver(head)
		if err != nil {
			// Cannot deliver — pop from outgoing and stall it.
			port.RetrieveOutgoing()
			if m.stalled == nil {
				m.stalled = make(map[int][]sim.Msg)
			}
			m.stalled[portIdx] = append(m.stalled[portIdx], head)
			continue
		}
		madeProgress = true
		port.RetrieveOutgoing()
	}

	return madeProgress
}

// StalledCount returns the total number of stalled messages for debugging.
func (m *middleware) StalledCount() int {
	total := 0
	for _, msgs := range m.stalled {
		total += len(msgs)
	}
	return total
}

// StalledCountForPort returns stalled messages for a specific port index.
func (m *middleware) StalledCountForPort(portIdx int) int {
	return len(m.stalled[portIdx])
}
