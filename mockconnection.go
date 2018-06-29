package core

import (
	"log"
)

type expectedReq struct {
	req Req
	err *SendError
}

// MockConnection provides an easy mock function to be used in the unit test
// system
type MockConnection struct {
	expectedReqs []*expectedReq
}

func (c *MockConnection) PlugIn(comp Connectable, port string) {
	comp.Connect(port, c)
}

func (c *MockConnection) Unplug(comp Connectable, port string) {
	comp.Disconnect(port)
}

func (c *MockConnection) NotifyAvailable(now VTimeInSec, comp Connectable) {
	panic("implement me")
}

// NewMockConnection returns a newly created MockConnection
func NewMockConnection() *MockConnection {
	c := new(MockConnection)
	c.expectedReqs = make([]*expectedReq, 0)
	return c
}

// ExpectSend register an req that is to be sent from the connection later.
// The send function will check if a request is expected or not.
func (c *MockConnection) ExpectSend(req Req, err *SendError) {
	c.expectedReqs = append(c.expectedReqs, &expectedReq{req, err})
}

// Send function of a MockConnection will check if the request is expected
func (c *MockConnection) Send(req Req) *SendError {
	if len(c.expectedReqs) == 0 {
		log.Panicf("Req %+v not expected", req)
	}

	match, reason := ReqEquivalent(req, c.expectedReqs[0].req)
	if match {
		err := c.expectedReqs[0].err
		c.expectedReqs = c.expectedReqs[1:]
		return err
	}

	log.Panic("Request not exptected: " + reason + "\n")

	return nil
}

func (c *MockConnection) dumpReq(req Req) {

}

// Handle function of a MockConnection does not do anything
func (c *MockConnection) Handle(evt Event) error {
	return nil
}

// AllExpectedSent determines if all the expected requested has been actually
// sent
func (c *MockConnection) AllExpectedSent() bool {
	return len(c.expectedReqs) == 0
}
