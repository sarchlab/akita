package requestsys

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
	return false
}

func (c *DirectConnection) Send(req *Request) {

}
