package event_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/event"
)

func TestEvent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Event System")
}

type MockEvent struct {
	EventTime       event.VTimeInSec
	EventHandler    event.Handler
	EventFinishChan chan bool
}

func NewMockEvent() *MockEvent {
	e := new(MockEvent)
	e.EventFinishChan = make(chan bool)
	return e
}

func (e *MockEvent) SetTime(time event.VTimeInSec) {
	e.EventTime = time
}

func (e MockEvent) Time() event.VTimeInSec {
	return e.EventTime
}

func (e *MockEvent) SetHandler(handler event.Handler) {
	e.EventHandler = handler
}

func (e MockEvent) Handler() event.Handler {
	return e.EventHandler
}

func (e MockEvent) FinishChan() chan bool {
	return e.EventFinishChan
}

type MockHandler struct {
	EventHandled []event.Event
	HandleFunc   func(event.Event)
}

func NewMockHandler() *MockHandler {
	return &MockHandler{make([]event.Event, 0), nil}
}

func (h *MockHandler) Handle(e event.Event) {
	h.EventHandled = append(h.EventHandled, e)
	if h.HandleFunc != nil {
		h.HandleFunc(e)
	}
}
