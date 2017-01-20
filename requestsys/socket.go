package requestsys

import (
	"errors"
	"fmt"
)

// A Socket is the communication outlet defined by a compoenent.
type Socket struct {
	Name      string
	Component Component
	conn      Connection
}

// NewSocket creates a socket for a certain compoenent
//
// The name of the socket will be set as ComponentName-SocketName
func NewSocket(component Component, name string) *Socket {
	return &Socket{component.Name() + name, component, nil}
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
	return s.conn.Send(req)
}

// Receive notifies the compoenent the arrival of the incomming request and
// let he compoenent to process the request
func (s *Socket) Receive(req *Request) error {
	return s.Component.Process(req)
}

// Connect put a connection into a socket
func (s *Socket) Connect(c Connection) error {
	if s.IsConnected() {
		s.Disconnect()
	}

	s.conn = c
	err := c.linkSocket(s)
	if err != nil {
		_ = fmt.Errorf("%s", err.Error)
		return err
	}
	return nil
}

// Disconnect remove the association between the socket and the current
// connected connection
func (s *Socket) Disconnect() error {
	if !s.IsConnected() {
		return errors.New("socket is not connected, cannot disconnect")
	}

	err := s.conn.unlinkSocket(s)
	if err != nil {
		_ = fmt.Errorf("%s", err.Error)
		return err
	}

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
