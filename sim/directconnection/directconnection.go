// Package directconnection provides directconnection
package directconnection

import (
	"github.com/sarchlab/akita/v3/sim"
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

// CanSend checks if the direct message can send a message from the port.
func (c *Comp) CanSend(src sim.Port) bool {
	c.Lock()
	defer c.Unlock()

	end := c.ends[src]

	canSend := len(end.buf) < end.bufSize

	if !canSend {
		end.busy = true
	}

	return canSend
}

// Send of a DirectConnection schedules a DeliveryEvent immediately
func (c *Comp) Send(msg sim.Msg) *sim.SendError {
	c.Lock()
	defer c.Unlock()

	c.msgMustBeValid(msg)

	srcEnd := c.ends[msg.Meta().Src]

	if len(srcEnd.buf) >= srcEnd.bufSize {
		srcEnd.busy = true
		return sim.NewSendError()
	}

	srcEnd.buf = append(srcEnd.buf, msg)

	c.TickNow(msg.Meta().SendTime)

	return nil
}

func (c *Comp) msgMustBeValid(msg sim.Msg) {
	c.portMustNotBeNil(msg.Meta().Src)
	c.portMustNotBeNil(msg.Meta().Dst)
	c.portMustBeConnected(msg.Meta().Src)
	c.portMustBeConnected(msg.Meta().Dst)
	c.srcDstMustNotBeTheSame(msg)
}

func (c *Comp) portMustNotBeNil(port sim.Port) {
	if port == nil {
		panic("src or dst is not given")
	}
}

func (c *Comp) portMustBeConnected(port sim.Port) {
	if _, connected := c.ends[port]; !connected {
		panic("src or dst is not connected")
	}
}

func (c *Comp) srcDstMustNotBeTheSame(msg sim.Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic("sending back to src")
	}
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
		if len(end.buf) == 0 {
			break
		}

		head := end.buf[0]
		head.Meta().RecvTime = now

		err := head.Meta().Dst.Recv(head)
		if err != nil {
			break
		}

		madeProgress = true
		end.buf = end.buf[1:]

		if end.busy {
			end.port.NotifyAvailable(now)
			end.busy = false
		}
	}

	return madeProgress
}
