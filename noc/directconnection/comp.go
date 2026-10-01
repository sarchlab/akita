// Package directconnection provides a connection that delivers messages
// between the ports plugged into it without latency.
package directconnection

import (
	"fmt"
	"io"
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec holds immutable configuration for the DirectConnection.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}

// State holds mutable runtime state for the DirectConnection.
type State struct {
	NextPortID int `json:"next_port_id"`
}

type ports struct {
	ports   []messaging.Port
	portMap map[messaging.RemotePort]int
}

func (p *ports) addPort(port messaging.Port) {
	p.ports = append(p.ports, port)
	p.portMap[port.AsRemote()] = len(p.ports) - 1
}

func (p *ports) getPortByName(name messaging.RemotePort) messaging.Port {
	portIndex, found := p.portMap[name]
	if !found {
		panic(fmt.Sprintf("port %s not found", name))
	}
	return p.ports[portIndex]
}

// Comp is a DirectConnection that connects components without latency. It is
// a connection, not a component: ports plug into it during wiring. It ticks on
// secondary tick events, so it runs after the components of the same cycle.
type Comp struct {
	hooking.HookableBase

	// State is the connection's mutable data, saved in checkpoints.
	State State

	lock  sync.Mutex
	name  string
	spec  Spec
	sim   timing.Simulation
	ticks *ticking.Scheduler
	ports ports
}

// Name returns the connection's name.
func (c *Comp) Name() string {
	return c.name
}

// NewID allocates an ID, unique within the connection's simulation.
func (c *Comp) NewID() uint64 {
	return c.sim.NewID()
}

// CurrentTime returns the simulation's current time.
func (c *Comp) CurrentTime() timing.VTimeInPicoSec {
	return c.ticks.CurrentTime()
}

// PlugIn marks the port connects to this DirectConnection.
func (c *Comp) PlugIn(port messaging.Port) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.ports.addPort(port)
	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *Comp) Unplug(_ messaging.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify the connection can deliver again.
func (c *Comp) NotifyAvailable(p messaging.Port) {
	for _, port := range c.ports.ports {
		if port == p {
			continue
		}
		port.NotifyAvailable()
	}
	c.ticks.TickNow()
}

// NotifySend is called by a port to notify the connection can start ticking.
func (c *Comp) NotifySend() {
	c.ticks.TickNow()
}

// Handle forwards messages on a tick and schedules the next tick if any
// message moved.
func (c *Comp) Handle(_ timing.Event) {
	if c.forward() {
		c.ticks.TickLater()
	}
}

// forward moves the messages waiting in the plugged-in ports to their
// destinations, starting from a round-robin port.
func (c *Comp) forward() bool {
	numPorts := len(c.ports.ports)
	madeProgress := false

	for i := range numPorts {
		portID := (i + c.State.NextPortID) % numPorts
		madeProgress = c.forwardMany(c.ports.ports[portID]) || madeProgress
	}

	c.State.NextPortID = (c.State.NextPortID + 1) % numPorts

	return madeProgress
}

func (c *Comp) forwardMany(port messaging.Port) bool {
	madeProgress := false
	for {
		head, ok := port.PeekOutgoing()
		if !ok {
			break
		}
		dst := head.Meta().Dst
		dstPort := c.ports.getPortByName(dst)
		if !dstPort.CanDeliver() {
			break
		}

		dstPort.Deliver(head)
		madeProgress = true
		port.RetrieveOutgoing()
	}
	return madeProgress
}

// SaveCheckpoint writes the connection's spec hash, State, and tick-scheduler
// guard.
func (c *Comp) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, c.spec, c.State, c.ticks)
}

// LoadCheckpoint restores the State and tick-scheduler guard after verifying
// that the saved spec hash matches this connection's.
func (c *Comp) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, c.spec, &c.State, c.ticks)
}
