package requestsys

import "errors"

// A Socket is the gateway for a component to communication with outside world.
// It provides an easy way for uses to rewire the connection from one
// component to another.
type Socket interface {
	Named
	Connectable

	Component() Component
	SetComponent(c Component) error
}

// A SimpleSocket is a naive implementation of the socket interface.
type SimpleSocket struct {
	name      string
	component Component
	conn      Connection
}

// NewSimpleSocket creates a new SimpleSocket object
func NewSimpleSocket(name string) *SimpleSocket {
	return &SimpleSocket{name, nil, nil}
}

// CanSend checks if the connection can send a certain request.
//
// There can be many different reasons why a request cannot be sent. For
// example, if the connection is a NetworkConn, this may means a buffer
// overfolow. If the connection si a DirectConn, the receiver component
// may be busy and cannot receive the request for now.
func (s *SimpleSocket) CanSend(req *Request) bool {
	if !s.IsConnected() {
		return false
	}

	return s.conn.CanSend(req)
}

// CanRecv checks if the compoenent that the socket belongs to can Process
// the request or not.
func (s *SimpleSocket) CanRecv(req *Request) bool {
	if s.component == nil {
		return false
	}
	return s.component.CanProcess(req)
}

// Send will deliever the request to its destination using the connected
// connection.
func (s *SimpleSocket) Send(req *Request) error {
	if s.conn == nil {
		return errors.New("socket is not connected to any connection")
	}
	return s.conn.Send(req)
}

// Recv notifies the compoenent the arrival of the incomming request and
// let he compoenent to process the request
func (s *SimpleSocket) Recv(req *Request) error {
	if s.component == nil {
		return errors.New("socket is not binded to any component")
	}

	return s.component.Process(req)
}

// Connect put a connection into a socket
func (s *SimpleSocket) Connect(c Connection) error {
	if s.IsConnected() {
		s.Disconnect()
	}

	s.conn = c
	_ = c.Register(s)
	return nil
}

// Disconnect remove the association between the socket and the current
// connected connection
func (s *SimpleSocket) Disconnect() error {
	if !s.IsConnected() {
		return errors.New("socket is not connected, cannot disconnect")
	}

	_ = s.conn.Unregister(s)
	s.conn = nil
	return nil
}

// IsConnected determines if a socket is connected with a connection
func (s *SimpleSocket) IsConnected() bool {
	if s.conn == nil {
		return false
	}
	return true
}

// Component returns the compoenent that the SimpleSocket is binded with
func (s *SimpleSocket) Component() Component {
	return s.component
}

// SetComponent sets the componet that the socket is binded with
func (s *SimpleSocket) SetComponent(c Component) error {
	if s.component != nil {
		return errors.New("the socket to component binding cannot be changed")
	}
	s.component = c
	return nil
}

// Name returns tha name of the socket
func (s *SimpleSocket) Name() string {
	return s.name
}
