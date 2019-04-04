package akita

import (
	"log"
	"sync"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestCore(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Akita")
}

type mockEvent struct {
	EventTime    VTimeInSec
	EventHandler Handler
}

func newMockEvent() *mockEvent {
	e := new(mockEvent)
	return e
}

func (e *mockEvent) SetTime(time VTimeInSec) {
	e.EventTime = time
}

func (e mockEvent) Time() VTimeInSec {
	return e.EventTime
}

func (e *mockEvent) SetHandler(handler Handler) {
	e.EventHandler = handler
}

func (e mockEvent) Handler() Handler {
	return e.EventHandler
}

type mockHandler struct {
	sync.Mutex
	EventHandled []Event
	HandleFunc   func(Event)
}

func newMockHandler() *mockHandler {
	h := new(mockHandler)
	h.EventHandled = make([]Event, 0)
	return h
}

func (h *mockHandler) Handle(e Event) error {
	h.Lock()
	defer h.Unlock()

	h.EventHandled = append(h.EventHandled, e)
	if h.HandleFunc != nil {
		h.HandleFunc(e)
	}
	return nil
}
