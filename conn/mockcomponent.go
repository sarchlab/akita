package conn

import (
	"log"

	"gitlab.com/yaotsu/core/event"
)

// A MockComponent is a very simple component that is designed only for
// simplify the unit tests.
type MockComponent struct {
	*BasicComponent

	ReceivedReqs  []Request
	ReceiveErrors []*Error
}

// NewMockComponent returns the a MockComponent
func NewMockComponent() *MockComponent {
	c := new(MockComponent)
	c.BasicComponent = NewBasicComponent("mock")
	return c
}

// Handle function of a MockComponent cannot handle any event
func (c *MockComponent) Handle(evt event.Event) error {
	return nil
}

// Receive of a MockComponent checks if a request is expected and returns a
// predefined error.
func (c *MockComponent) Receive(req Request) *Error {
	if len(c.ReceivedReqs) == 0 || c.ReceivedReqs[0] != req {
		log.Panicf("Request not expected %+v", req)
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
func (c *MockComponent) ToReceiveReq(req Request, err *Error) {
	c.ReceiveErrors = append(c.ReceiveErrors, err)
	c.ReceivedReqs = append(c.ReceivedReqs, req)
}
