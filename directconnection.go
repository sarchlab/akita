package core

import (
	"log"
	"sync"
)

// DirectConnection connects two components without latency
type DirectConnection struct {
	sync.Mutex

	endPoints    map[*Port]bool
	reqBuf       map[*Port][]Req
	receiverBusy map[*Port]bool

	engine Engine
}

func (c *DirectConnection) PlugIn(port *Port) {
	c.Lock()
	defer c.Unlock()

	c.endPoints[port] = true
	c.receiverBusy[port] = false
	c.reqBuf[port] = make([]Req, 0)
	port.Conn = c
}

func (c *DirectConnection) Unplug(port *Port) {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.endPoints[port]; !ok {
		log.Panicf("connectable if not attached")
	}

	delete(c.endPoints, port)
	delete(c.reqBuf, port)
	delete(c.receiverBusy, port)
	port.Conn = nil
}

func (c *DirectConnection) NotifyAvailable(now VTimeInSec, port *Port) {
	//c.Lock()
	//defer c.Unlock()

	//if c.receiverBusy[port] == false {
	//	return
	//}
	//
	//c.receiverBusy[port] = false
	//buf := c.reqBuf[port]
	//if len(buf) > 0 {
	//	evt := NewDeliverEvent(now, c, buf[0])
	//	c.engine.Schedule(evt)
	//}

	for p := range c.endPoints {
		p.NotifyAvailable(now)
	}

}

// Send of a DirectConnection schedules a DeliveryEvent immediately
func (c *DirectConnection) Send(req Req) *SendError {
	//c.Lock()
	//defer c.Unlock()
	//
	//dst := req.Dst()
	//buf := c.reqBuf[dst]
	//buf = append(buf, req)
	//c.reqBuf[dst] = buf
	//
	//if c.receiverBusy[dst] || len(buf) > 1 {
	//	return nil
	//}
	//
	//evt := NewDeliverEvent(req.SendTime(), c, req)
	//c.engine.Schedule(evt)
	//return nil

	req.SetRecvTime(req.SendTime())
	return req.Dst().Recv(req)
}

// Handle defines how the DirectConnection handles events
func (c *DirectConnection) Handle(evt Event) error {
	c.Lock()
	defer c.Unlock()

	switch evt := evt.(type) {
	case *DeliverEvent:
		return c.handleDeliverEvent(evt)
	}
	return nil
}

func (c *DirectConnection) handleDeliverEvent(evt *DeliverEvent) error {
	now := evt.Time()
	req := evt.Req
	req.SetRecvTime(evt.Time())
	dst := req.Dst()
	buf := c.reqBuf[dst]

	// Deliver
	err := dst.Recv(req)
	if err != nil {
		c.receiverBusy[dst] = true
		return nil
	}
	buf = buf[1:]

	// Schedule more delivery
	if len(buf) > 0 {
		evt := NewDeliverEvent(now, c, buf[0])
		c.engine.Schedule(evt)
	}

	c.reqBuf[dst] = buf
	return nil
}

// NewDirectConnection creates a new DirectConnection object
func NewDirectConnection(engine Engine) *DirectConnection {
	c := DirectConnection{}

	c.endPoints = make(map[*Port]bool)
	c.receiverBusy = make(map[*Port]bool)
	c.reqBuf = make(map[*Port][]Req)

	c.engine = engine
	return &c
}
