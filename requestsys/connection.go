package requestsys

// A Connectable is an object that an Connection can connect with.
//
// The only connectable object is Socket so far
type Connectable interface {
	IsConnected() bool

	// Connect to a connection. This function should invoke the Connection's
	// Register function.
	Connect(conn Connection) error
	Disconnect() error

	Sender
	Receiver
}

// A Connection is responsible for delievering the requests to its destination.
type Connection interface {
	Sender

	Register(s Connectable) error
	Unregister(s Connectable) error
}

// BasicConn is dummy implementation of the connection providing some utilities
// that all other type of connections can use
type BasicConn struct {
	sockets map[Component]Socket
}

// NewBasicConn creates a basic connection object
func NewBasicConn() *BasicConn {
	c := BasicConn{make(map[Component]Socket)}
	return &c
}

// Register adds a Connectable object in the connected list
func (c *BasicConn) Register(s Connectable) error {
	c.sockets[s] = true
	return nil
}

// Unregister removes a Connectable object from the connected list
func (c *BasicConn) Unregister(s Connectable) error {
	delete(c.sockets, s)
	return nil
}
