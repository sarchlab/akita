package eventsys_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/eventsys"
)

var toHappen []*testEvent
var happened []*testEvent

type testEvent struct {
	time eventsys.VTimeInSec
}

func (e *testEvent) Happen() {
	happened = append(happened, e)
}

func (e *testEvent) Time() eventsys.VTimeInSec {
	return e.time
}

func (e *testEvent) SetTime(time eventsys.VTimeInSec) {
	e.time = time
}

var _ = Describe("Engine", func() {
	var engine *eventsys.Engine

	BeforeEach(func() {
		toHappen = make([]*testEvent, 0)
		happened = make([]*testEvent, 0)
		engine = eventsys.NewEngine()
	})

	It("should start with no event", func() {
		Expect(engine.HasMoreEvent()).To(Equal(false))
	})

	It("should schedule event", func() {
		e := new(testEvent)

		engine.Schedule(e, 10)
		Expect(engine.HasMoreEvent()).To(Equal(true))

		engine.Run()

		toHappen = append(toHappen, e)
		Expect(len(happened)).To(Equal(len(toHappen)))
		for i, event := range happened {
			Expect(event).To(Equal(toHappen[i]))
		}

		Expect(engine.Now()).To(Equal(eventsys.VTimeInSec(10.0)))
	})

	It("should execute in time order", func() {

		e1 := new(testEvent)
		e2 := new(testEvent)
		e3 := new(testEvent)
		e4 := new(testEvent)

		engine.Schedule(e1, 0)
		engine.Schedule(e2, 10)
		engine.Schedule(e3, 10)

		engine.Run()
		Expect(engine.Now()).To(Equal(eventsys.VTimeInSec(0.0)))

		engine.Run()
		Expect(engine.Now()).To(Equal(eventsys.VTimeInSec(10.0)))

		engine.Run()
		Expect(engine.Now()).To(Equal(eventsys.VTimeInSec(10.0)))

		engine.Schedule(e4, 100)
		engine.Run()
		Expect(engine.Now()).To(Equal(eventsys.VTimeInSec(110.0)))
	})
})
