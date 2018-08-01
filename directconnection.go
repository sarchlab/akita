package core

import (
	"log"
	"sync"
)

// DirectConnection connects two components without latency
type DirectConnection struct {
	sync.Mutex

	endPoints map[*Port]bool
	engine    Engine
}

func (c *DirectConnection) PlugIn(port *Port) {
	c.Lock()
	defer c.Unlock()

	c.endPoints[port] = true
	port.Conn = c
}

func (c *DirectConnection) Unplug(port *Port) {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.endPoints[port]; !ok {
		log.Panicf("connectable if not attached")
	}

	delete(c.endPoints, port)
	port.Conn = nil
}

func (c *DirectConnection) NotifyAvailable(now VTimeInSec, port *Port) {
	for p := range c.endPoints {
		p.NotifyAvailable(now)
	}
}

// Send of a DirectConnection schedules a DeliveryEvent immediately
func (c *DirectConnection) Send(req Req) *SendError {
	if _, found := c.endPoints[req.Dst()]; !found {
		log.Panicf("destination %s not connected, "+
			"req ID %s, "+
			"request from %s",
			req.Dst().Comp.Name(),
			req.GetID(),
			req.Dst().Comp.Name(),
		)
	}

	req.SetRecvTime(req.SendTime())
	return req.Dst().Recv(req)
}

// Handle defines how the DirectConnection handles events
func (c *DirectConnection) Handle(evt Event) error {
	return nil
}

// NewDirectConnection creates a new DirectConnection object
func NewDirectConnection(engine Engine) *DirectConnection {
	c := DirectConnection{}

	c.endPoints = make(map[*Port]bool)

	c.engine = engine
	return &c
}
