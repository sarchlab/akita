package eventsys_test

import (
	"gitlab.com/syifan/yaotsu/eventsys"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var called int

type testHandler struct{}

func (t *testHandler) Handle(e eventsys.Event) {
	called++
}

var _ = Describe("HandledEvent", func() {
	It("should allow no handler", func() {
		called = 0
		e := eventsys.NewHandledEvent()
		e.Happen()
		Expect(called).To(Equal(0))

	})

	It("should allow one handler", func() {
		called = 0
		e := eventsys.NewHandledEvent()
		e.AddHandler(new(testHandler))
		e.Happen()
		Expect(called).To(Equal(1))
	})

	It("should allow multiple handlers", func() {
		called = 0
		e := eventsys.NewHandledEvent()
		e.AddHandler(new(testHandler))
		e.AddHandler(new(testHandler))
		e.AddHandler(new(testHandler))
		e.Happen()
		Expect(called).To(Equal(3))
	})

})
