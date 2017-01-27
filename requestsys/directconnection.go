package requestsys

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
func (c *DirectConnection) CanSend(req *Request) bool {
	dst, err := c.getDest(req)
	if err != nil {
		_ = fmt.Errorf("%v", err)
		return false
	}

	return dst.CanRecv(req)
}

func (c *DirectConnection) Send(req *Request) error {
	return nil

}
