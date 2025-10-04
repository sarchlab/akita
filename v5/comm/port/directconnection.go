package port

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/v5/comm"
	hooking "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"
)

type portRegistry struct {
	ports   []comm.Port
	portMap map[comm.RemotePort]int
}

func newPortRegistry() portRegistry {
	return portRegistry{
		ports:   make([]comm.Port, 0, 4),
		portMap: make(map[comm.RemotePort]int),
	}
}

func (r *portRegistry) add(port comm.Port) {
	r.ports = append(r.ports, port)
	r.portMap[port.AsRemote()] = len(r.ports) - 1
}

func (r *portRegistry) byIndex(index int) comm.Port {
	return r.ports[index]
}

func (r *portRegistry) byName(name comm.RemotePort) comm.Port {
	idx, ok := r.portMap[name]
	if !ok {
		panic(fmt.Sprintf("port %s not found", name))
	}

	return r.ports[idx]
}

func (r *portRegistry) list() []comm.Port {
	return r.ports
}

func (r *portRegistry) len() int {
	return len(r.ports)
}

// DirectConnection connects multiple ports without latency.
type DirectConnection struct {
	*hooking.HookableBase

	lock sync.Mutex
	name string

	ports      portRegistry
	nextPortID int
}

var _ comm.Connection = (*DirectConnection)(nil)

// NewDirectConnection creates an instance with the provided name.
func NewDirectConnection(name string) *DirectConnection {
	return &DirectConnection{
		HookableBase: hooking.NewHookableBase(),
		name:         name,
		ports:        newPortRegistry(),
	}
}

// Name implements comm.Named.
func (c *DirectConnection) Name() string {
	return c.name
}

// PlugIn attaches a port to the connection.
func (c *DirectConnection) PlugIn(port comm.Port) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if port == nil {
		panic("nil port")
	}

	c.ports.add(port)
	port.SetConnection(c)
}

// Unplug removes a port from the connection.
func (c *DirectConnection) Unplug(_ comm.Port) {
	panic("directconnection: unplug not implemented")
}

// NotifyAvailable notifies the connection that a port regained capacity.
func (c *DirectConnection) NotifyAvailable(comm.Port) {
	c.tick()
}

// NotifySend notifies the connection that a port has messages ready.
func (c *DirectConnection) NotifySend() {
	c.tick()
}

func (c *DirectConnection) tick() {
	c.lock.Lock()
	defer c.lock.Unlock()

	n := c.ports.len()
	if n == 0 {
		return
	}

	madeProgress := false

	for i := 0; i < n; i++ {
		portID := (i + c.nextPortID) % n
		port := c.ports.byIndex(portID)
		if c.forwardMany(port) {
			madeProgress = true
		}
	}

	if madeProgress {
		c.nextPortID = (c.nextPortID + 1) % n
	}
}

func (c *DirectConnection) forwardMany(port comm.Port) bool {
	madeProgress := false

	for {
		head := port.PeekOutgoing()
		if head == nil {
			break
		}

		dst := head.Dst()
		dstPort := c.ports.byName(dst)

		err := dstPort.Deliver(head)
		if err != nil {
			break
		}

		madeProgress = true

		port.RetrieveOutgoing()
	}

	return madeProgress
}
