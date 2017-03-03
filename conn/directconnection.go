package conn

import "log"

// DirectConnection provides a way to connect two component directly so that
// no latency would happen.
type DirectConnection struct {
	EndPoints map[Connectable]bool
}

// NewDirectConnection creates a new DirectConnection object
func NewDirectConnection() *DirectConnection {
	c := DirectConnection{}
	c.EndPoints = make(map[Connectable]bool)
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
func (c *DirectConnection) Send(req Request) *Error {
	if req.Source() == nil {
		return NewError("source of a request is nil", false, 0)
	}

	if _, ok := c.EndPoints[req.Source()]; !ok {
		return NewError("source of is not connected on this connection", false, 0)
	}

	if req.Destination() == nil {
		return NewError("destination of a request is nil", false, 0)
	}

	if _, ok := c.EndPoints[req.Destination()]; !ok {
		return NewError("destination is not connected on this connection", false, 0)
	}

	req.SetRecvTime(req.SendTime())
	return req.Destination().Receive(req)
}
