// Package direct provides a connection that delivers messages
// between the ports plugged into it without latency.
package direct

import (
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// configuration retains the immutable frequency for checkpoint validation.
type configuration struct {
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

func (p *ports) addPort(port messaging.Port, name messaging.RemotePort) {
	p.ports = append(p.ports, port)
	p.portMap[name] = len(p.ports) - 1
}

func (p *ports) getPortByName(name messaging.RemotePort) messaging.Port {
	portIndex, found := p.portMap[name]
	if !found {
		panic(fmt.Sprintf("port %s not found", name))
	}
	return p.ports[portIndex]
}

// Connection is an ideal connection that connects components without latency. It is
// a connection, not a component: ports plug into it during wiring. It ticks on
// secondary tick events, so it runs after the components of the same cycle.
type Connection struct {
	hooking.HookableBase

	// State is the connection's mutable data, saved in checkpoints.
	State State

	lock       sync.Mutex
	name       string
	simulation timing.Simulation
	spec       configuration
	ticks      *ticking.Scheduler
	ports      ports
}

// Name returns the connection's name.
func (c *Connection) Name() string {
	return c.name
}

// BindPort adds a port and establishes its reverse connection attachment.
func (c *Connection) BindPort(port messaging.Port) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.simulation.RequireSetup()
	if port == nil || (reflect.ValueOf(port).Kind() == reflect.Pointer && reflect.ValueOf(port).IsNil()) {
		panic("direct: cannot bind a nil port")
	}
	if port.Owner() == nil {
		panic("direct: bind the port owner first")
	}
	if port.Connection() != nil {
		panic("direct: port already has a connection")
	}
	name := port.AsRemote()
	if _, exists := c.ports.portMap[name]; exists {
		panic("direct: duplicate port name")
	}
	port.BindConnection(c)
	c.ports.addPort(port, name)
}

// NotifyAvailable is called by a port to notify the connection can deliver again.
func (c *Connection) NotifyAvailable(p messaging.Port) {
	for _, port := range c.ports.ports {
		if port == p {
			continue
		}
		port.NotifyAvailable()
	}
	c.ticks.TickNow()
}

// NotifySend is called by a port to notify the connection can start ticking.
func (c *Connection) NotifySend() {
	c.ticks.TickNow()
}

// Handle forwards messages on a tick and schedules the next tick if any
// message moved.
func (c *Connection) Handle(_ timing.Event) {
	if c.forward() {
		c.ticks.TickLater()
	}
}

// forward moves the messages waiting in the plugged-in ports to their
// destinations, starting from a round-robin port.
func (c *Connection) forward() bool {
	numPorts := len(c.ports.ports)
	madeProgress := false

	for i := range numPorts {
		portID := (i + c.State.NextPortID) % numPorts
		madeProgress = c.forwardMany(c.ports.ports[portID]) || madeProgress
	}

	c.State.NextPortID = (c.State.NextPortID + 1) % numPorts

	return madeProgress
}

func (c *Connection) forwardMany(port messaging.Port) bool {
	madeProgress := false
	for {
		head, ok := port.PeekOutgoing()
		if !ok {
			break
		}
		dst := head.Dst
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
func (c *Connection) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, c.spec, c.State, c.ticks)
}

// LoadCheckpoint restores the State and tick-scheduler guard after verifying
// that the saved spec hash matches this connection's.
func (c *Connection) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, c.spec, &c.State, c.ticks)
}

// RequireSetup rejects connection changes after simulation initialization.
func (c *Connection) RequireSetup() { c.simulation.RequireSetup() }
