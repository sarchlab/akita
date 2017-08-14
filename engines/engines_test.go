package engines

import (
	"log"
	"sync"
	"testing"

	"gitlab.com/yaotsu/core"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestEngines(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Yaotsu Engines")
}

type MockEvent struct {
	EventTime    core.VTimeInSec
	EventHandler core.Handler
}

func NewMockEvent() *MockEvent {
	e := new(MockEvent)
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

type MockHandler struct {
	sync.Mutex
	EventHandled []core.Event
	HandleFunc   func(core.Event)
}

func NewMockHandler() *MockHandler {
	h := new(MockHandler)
	h.EventHandled = make([]core.Event, 0)
	return h
}

func (h *MockHandler) Handle(e core.Event) error {
	h.Lock()
	defer h.Unlock()

	h.EventHandled = append(h.EventHandled, e)
	if h.HandleFunc != nil {
		h.HandleFunc(e)
	}
	return nil
}
