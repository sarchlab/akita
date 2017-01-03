package event

import "testing"

var called int32

type TestHandler struct{}

func (t *TestHandler) Handle(e Event) {
	called++
	println("handling")
}

func TestHandledEvent(t *testing.T) {
	t.Run("no_handler", func(t *testing.T) {
		called = 0
		e := NewHandledEvent(0)
		e.Happen()
		if called > 0 {
			t.FailNow()
		}
	})

	t.Run("one_handler", func(t *testing.T) {
		called = 0
		e := NewHandledEvent(0)
		e.AddHandler(new(TestHandler))
		e.Happen()
		if called != 1 {
			t.FailNow()
		}
	})

	t.Run("three_handler", func(t *testing.T) {
		called = 0
		e := NewHandledEvent(0)
		e.AddHandler(new(TestHandler))
		e.AddHandler(new(TestHandler))
		e.AddHandler(new(TestHandler))
		e.Happen()
		if called != 3 {
			t.FailNow()
		}
	})

}
