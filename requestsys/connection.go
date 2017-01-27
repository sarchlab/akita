package requestsys

// A Sender can send requests to their destinations
type Sender interface {
	CanSend(req *Request) bool
	Send(req *Request) error
}

// A Receiver can receive requests
type Receiver interface {
	CanRecv(req *Request) bool
	Recv(req *Request) error
}

// A Connectable is an object that an Connection can connect with.
type Connectable interface {
	Connect(portName string, conn Connection) error
	GetConnection(portName string) Connection
	Disconnect(portName string) error

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
	connectables map[Connectable]bool
}

// NewBasicConn creates a basic connection object
func NewBasicConn() *BasicConn {
	c := BasicConn{make(map[Connectable]bool)}
	return &c
}

// Register adds a Connectable object in the connected list
func (c *BasicConn) Register(s Connectable) error {
	c.connectables[s] = true
	return nil
}

// Unregister removes a Connectable object from the connected list
func (c *BasicConn) Unregister(s Connectable) error {
	delete(c.connectables, s)
	return nil
}
