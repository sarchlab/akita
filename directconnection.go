package core

import (
	"log"
	"sync"
)

// DirectConnection connects two components without latency
type DirectConnection struct {
	sync.Mutex

	endPoints    map[Connectable]bool
	reqBuf       map[Connectable][]Req
	receiverBusy map[Connectable]bool

	engine Engine
}

func (c *DirectConnection) PlugIn(comp Connectable, port string) {
	c.Lock()
	defer c.Unlock()

	c.endPoints[comp] = true
	c.receiverBusy[comp] = false
	c.reqBuf[comp] = make([]Req, 0)
	comp.Connect(port, c)
}

func (c *DirectConnection) Unplug(comp Connectable, port string) {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.endPoints[comp]; !ok {
		log.Panicf("connectable if not attached")
	}

	delete(c.endPoints, comp)
	delete(c.reqBuf, comp)
	delete(c.receiverBusy, comp)
	comp.Disconnect(port)
}

func (c *DirectConnection) NotifyAvailable(now VTimeInSec, comp Connectable) {
	c.Lock()
	defer c.Unlock()

	c.receiverBusy[comp] = false
	buf := c.reqBuf[comp]
	if len(buf) > 0 {
		evt := NewDeliverEvent(now, c, buf[0])
		c.engine.Schedule(evt)
	}
}

// Send of a DirectConnection schedules a DeliveryEvent immediately
func (c *DirectConnection) Send(req Req) *SendError {
	c.Lock()
	defer c.Unlock()

	dst := req.Dst()
	buf := c.reqBuf[dst]
	buf = append(buf, req)
	c.reqBuf[dst] = buf

	if c.receiverBusy[dst] || len(buf) > 1 {
		return nil
	}

	evt := NewDeliverEvent(req.SendTime(), c, req)
	c.engine.Schedule(evt)
	return nil
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

	c.endPoints = make(map[Connectable]bool)
	c.receiverBusy = make(map[Connectable]bool)
	c.reqBuf = make(map[Connectable][]Req)

	c.engine = engine
	return &c
}
