package requestsys

import "errors"

// A Socket is the gateway for a component to communication with outside world.
// It provides an easy way for uses to rewire the connection from one
// component to another.
type Socket struct {
	Name      string
	Component Component
	conn      Connection
}

// NewSocket creates a new socket object
func NewSocket(name string) *Socket {
	return &Socket{name, nil, nil}
}

// CanSend checks if the connection can send a certain request.
//
// There can be many different reasons why a request cannot be sent. For
// example, if the connection is a NetworkConn, this may means a buffer
// overfolow. If the connection si a DirectConn, the receiver component
// may be busy and cannot receive the request for now.
func (s *Socket) CanSend(req *Request) bool {
	if !s.IsConnected() {
		return false
	}

	return s.conn.CanSend(req)
}

// CanReceive checks if the compoenent that the socket belongs to can Process
// the request or not.
func (s *Socket) CanReceive(req *Request) bool {
	if s.Component == nil {
		return false
	}
	return s.Component.CanProcess(req)
}

// Send will deliever the request to its destination using the connected
// connection.
func (s *Socket) Send(req *Request) error {
	if s.conn == nil {
		return errors.New("socket is not connected to any connection")
	}
	return s.conn.Send(req)
}

// Receive notifies the compoenent the arrival of the incomming request and
// let he compoenent to process the request
func (s *Socket) Receive(req *Request) error {
	if s.Component == nil {
		return errors.New("socket is not binded to any component")
	}

	return s.Component.Process(req)
}

// Connect put a connection into a socket
func (s *Socket) Connect(c Connection) error {
	if s.IsConnected() {
		s.Disconnect()
	}

	s.conn = c
	_ = c.linkSocket(s)
	return nil
}

// Disconnect remove the association between the socket and the current
// connected connection
func (s *Socket) Disconnect() error {
	if !s.IsConnected() {
		return errors.New("socket is not connected, cannot disconnect")
	}

	_ = s.conn.unlinkSocket(s)
	s.conn = nil
	return nil
}

// IsConnected determines if a socket is connected with a connection
func (s *Socket) IsConnected() bool {
	if s.conn == nil {
		return false
	}
	return true
}
