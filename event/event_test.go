package event_test

import (
	"testing"

	"gitlab.com/yaotsu/core/event"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEvent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Event System")
}

var called int

type testHandler struct{}

func (t *testHandler) Handle(e event.Event) {
	called++
}

var _ = Describe("HandledEvent", func() {
	It("should allow no handler", func() {
		called = 0
		e := event.NewHandledEvent()
		e.Happen()
		Expect(called).To(Equal(0))

	})

	It("should allow one handler", func() {
		called = 0
		e := event.NewHandledEvent()
		e.AddHandler(new(testHandler))
		e.Happen()
		Expect(called).To(Equal(1))
	})

	It("should allow multiple handlers", func() {
		called = 0
		e := event.NewHandledEvent()
		e.AddHandler(new(testHandler))
		e.AddHandler(new(testHandler))
		e.AddHandler(new(testHandler))
		e.Happen()
		Expect(called).To(Equal(3))
	})

})
