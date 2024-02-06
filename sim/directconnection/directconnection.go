// Package directconnection provides directconnection
package directconnection

import (
	"github.com/sarchlab/akita/v4/sim"
)

type directConnectionEnd struct {
	port    sim.Port
	buf     []sim.Msg
	bufSize int
	busy    bool
}

// Comp is a DirectConnection connects two components without latency
type Comp struct {
	*sim.TickingComponent

	nextPortID int
	ports      []sim.Port
	ends       map[sim.Port]*directConnectionEnd
}

// PlugIn marks the port connects to this DirectConnection.
func (c *Comp) PlugIn(port sim.Port, sourceSideBufSize int) {
	c.Lock()
	defer c.Unlock()

	c.ports = append(c.ports, port)
	end := &directConnectionEnd{}
	end.port = port
	end.bufSize = sourceSideBufSize
	c.ends[port] = end

	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify that the connection can
// deliver to the port again.
func (c *Comp) NotifyAvailable(now sim.VTimeInSec, _ sim.Port) {
	c.TickNow(now)
}

// NotifySend is called by a port to notify that the connection can start
// to tick now
func (c *Comp) NotifySend(now sim.VTimeInSec) {
	c.TickNow(now)
}

// Tick updates the states of the connection and delivers messages.
func (c *Comp) Tick(now sim.VTimeInSec) bool {
	madeProgress := false
	for i := 0; i < len(c.ports); i++ {
		portID := (i + c.nextPortID) % len(c.ports)
		port := c.ports[portID]
		end := c.ends[port]
		madeProgress = c.forwardMany(end, now) || madeProgress
	}

	c.nextPortID = (c.nextPortID + 1) % len(c.ports)
	return madeProgress
}

func (c *Comp) forwardMany(
	end *directConnectionEnd,
	now sim.VTimeInSec,
) bool {
	madeProgress := false
	for {
		if end.port.PeekOutgoing() == nil {
			break
		}

		head := end.port.PeekOutgoing()
		head.Meta().RecvTime = now

		err := head.Meta().Dst.Deliver(head)
		if err != nil {
			break
		}

		madeProgress = true
		end.port.RetrieveOutgoing()

		if end.busy {
			end.port.NotifyAvailable(now)
			end.busy = false
		}
	}

	return madeProgress
}
