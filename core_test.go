package core_test

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

func TestCore(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Yaotsu Core")
}

type MockConnection struct {
	Connected map[core.Connectable]bool
	ReqSent   []core.Request
}

func NewMockConnection() *MockConnection {
	return &MockConnection{
		make(map[core.Connectable]bool),
		make([]core.Request, 0)}
}

func (c *MockConnection) Attach(connectable core.Connectable) {
	c.Connected[connectable] = true
}

func (c *MockConnection) Detach(connectable core.Connectable) {
	c.Connected[connectable] = false
}

func (c *MockConnection) Send(req core.Request) *core.Error {
	c.ReqSent = append(c.ReqSent, req)
	return nil
}

type MockRequest struct {
	*core.BasicRequest
}

func NewMockRequest() *MockRequest {
	return &MockRequest{core.NewBasicRequest()}
}

type MockEvent struct {
	EventTime       core.VTimeInSec
	EventHandler    core.Handler
	EventFinishChan chan bool
}

func NewMockEvent() *MockEvent {
	e := new(MockEvent)
	e.EventFinishChan = make(chan bool)
	return e
}

func (e *MockEvent) SetTime(time core.VTimeInSec) {
	e.EventTime = time
}

func (e MockEvent) Time() core.VTimeInSec {
	return e.EventTime
}

func (e *MockEvent) SetHandler(handler core.Handler) {
	e.EventHandler = handler
}

func (e MockEvent) Handler() core.Handler {
	return e.EventHandler
}

func (e MockEvent) FinishChan() chan bool {
	return e.EventFinishChan
}

type MockHandler struct {
	EventHandled []core.Event
	HandleFunc   func(core.Event)
}

func NewMockHandler() *MockHandler {
	return &MockHandler{make([]core.Event, 0), nil}
}

func (h *MockHandler) Handle(e core.Event) error {
	h.EventHandled = append(h.EventHandled, e)
	if h.HandleFunc != nil {
		h.HandleFunc(e)
	}
	return nil
}
