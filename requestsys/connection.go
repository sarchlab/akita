package requestsys

// A Connection is responsible for delievering the requests to its destination.
type Connection interface {
	Sender

	// linkSocket and unlinkSocket are not exported, because they should only
	// be called from the socket. Users should use socket.Plugin and
	// socket.Disconnect only
	linkSocket(s *Socket) error
	unlinkSocket(s *Socket) error
}

// BasicConn is dummy implementation of the connection providing some utilities
// that all other type of connections can use
type BasicConn struct {
	sockets map[*Socket]bool
}

// NewBasicConn creates a basic connection object
func NewBasicConn() *BasicConn {
	c := BasicConn{make(map[*Socket]bool)}
	return &c
}

// linkSocket adds a socket into the connected socket list
func (c *BasicConn) linkSocket(s *Socket) error {
	c.sockets[s] = true
	return nil
}

// unlinkSocket removes a socket from the list of sockets in the connection
func (c *BasicConn) unlinkSocket(s *Socket) error {
	delete(c.sockets, s)
	return nil
}
