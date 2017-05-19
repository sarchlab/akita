package core

import "log"

// DirectConnection provides a way to connect two component directly so that
// no latency would happen.
type DirectConnection struct {
	EndPoints map[Connectable]bool

	engine Engine
}

// NewDirectConnection creates a new DirectConnection object
func NewDirectConnection(engine Engine) *DirectConnection {
	c := DirectConnection{}
	c.EndPoints = make(map[Connectable]bool)
	c.engine = engine
	return &c
}

// Attach adds a Connectable object into the end point list of the
// DirectConnection.
func (c *DirectConnection) Attach(connectable Connectable) {
	c.EndPoints[connectable] = true
}

// Detach removes a Connectable from the end point list of the
// DirectConnection
func (c *DirectConnection) Detach(connectable Connectable) {
	if _, ok := c.EndPoints[connectable]; !ok {
		log.Panicf("connectable if not attached")
	}

	delete(c.EndPoints, connectable)
}

// Send of a DirectConnection invokes receiver's Recv method
func (c *DirectConnection) Send(req Req) *Error {
	if req.Src() == nil {
		return NewError("source of a request is nil", false, 0)
	}

	if _, ok := c.EndPoints[req.Src()]; !ok {
		return NewError("source of is not connected on this connection", false, 0)
	}

	if req.Dst() == nil {
		return NewError("destination of a request is nil", false, 0)
	}

	if _, ok := c.EndPoints[req.Dst()]; !ok {
		return NewError("destination is not connected on this connection", false, 0)
	}

	req.SetRecvTime(req.SendTime())
	return req.Dst().Recv(req)
}

// Handle defines how the DirectConnection handles events
func (c *DirectConnection) Handle(evt Event) error {
	return nil
}
