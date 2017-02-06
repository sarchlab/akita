package conn

import (
	"fmt"
)

// DirectConnection provides a way to connect two component directly so that
// no latency would happen.
type DirectConnection struct {
	*BasicConn
}

// NewDirectConnection creates a new DirectConnection object
func NewDirectConnection() *DirectConnection {
	c := DirectConnection{NewBasicConn()}
	return &c
}

// CanSend of the DirectConnection only checks if the receiver can process the
// request.
func (c *DirectConnection) CanSend(req Request) *Error {
	_, ok := c.BasicConn.connectables[req.Source()]
	if !ok {
		return NewError("Source "+req.Source().Name()+" is not connected",
			false, 0)
	}

	dst, err := c.getDest(req)
	if err != nil {
		_ = fmt.Errorf("%v", err)
		return NewError(err.Error(), false, 0)
	}

	return dst.CanRecv(req)
}

// Send of a DirectConnection invokes receiver's Recv method
func (c *DirectConnection) Send(req Request) *Error {
	if req.Destination() == nil {
		return NewError("Destination of a request is not known.", false, 0)
	}
	return req.Destination().Recv(req)
}
