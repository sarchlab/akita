package core

import (
	"log"
)

// A MockComponent is a very simple component that is designed only for
// simplify the unit tests.
type MockComponent struct {
	*BasicComponent

	ReceivedReqs  []Req
	ReceiveErrors []*Error
}

// NewMockComponent returns the a MockComponent
func NewMockComponent(name string) *MockComponent {
	c := new(MockComponent)
	c.BasicComponent = NewBasicComponent(name)
	return c
}

// Handle function of a MockComponent cannot handle any event
func (c *MockComponent) Handle(evt Event) error {
	return nil
}

// Recv of a MockComponent checks if a request is expected and returns a
// predefined error.
func (c *MockComponent) Recv(req Req) *Error {
	if len(c.ReceivedReqs) == 0 {
		log.Panicln("No request is expected")
	}
	if c.ReceivedReqs[0] != req {
		log.Panicln("Request not expected")
	}
	if len(c.ReceiveErrors) == 0 {
		log.Panic("Not sure what error to return")
	}

	err := c.ReceiveErrors[0]

	c.ReceiveErrors = c.ReceiveErrors[1:]
	c.ReceivedReqs = c.ReceivedReqs[1:]

	return err
}

// ToReceiveReq defines the request that a mock component is going to receive
// during the test and the errors to return
func (c *MockComponent) ToReceiveReq(req Req, err *Error) {
	c.ReceiveErrors = append(c.ReceiveErrors, err)
	c.ReceivedReqs = append(c.ReceivedReqs, req)
}

// AllReqReceived returns true if all the expected requests has been received
func (c *MockComponent) AllReqReceived() bool {
	return len(c.ReceivedReqs) == 0
}
