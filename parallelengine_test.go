package akita

import (
	"log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParallelEngine", func() {
	var (
		engine *ParallelEngine
	)

	BeforeEach(func() {
		engine = NewParallelEngine()
	})

	It("should schedule events", func() {
		handler1 := newMockHandler()
		handler2 := newMockHandler()
		handler3 := newMockHandler()
		evt1 := newMockEvent()
		evt2 := newMockEvent()
		evt3 := newMockEvent()
		evt4 := newMockEvent()

		// Four events to be scheduled. Evt1 and Evt2 are directly scheduled,
		// while evt2 schdules evt3 and evt4. They should be executed
		// in the global time order
		evt1.SetTime(4.0)
		evt1.SetHandler(handler1)
		evt2.SetTime(2.0)
		evt2.SetHandler(handler2)
		evt3.SetTime(3.0)
		evt3.SetHandler(handler3)
		evt4.SetTime(3.0)
		evt4.SetHandler(handler1)

		handler1.HandleFunc = func(e Event) {
			log.Printf("Handled %f\n", e.Time())
		}
		handler2.HandleFunc = func(e Event) {
			engine.Schedule(evt3)
			engine.Schedule(evt4)
			log.Printf("Handled %f\n", e.Time())
		}

		engine.Schedule(evt1)
		engine.Schedule(evt2)

		engine.Run()

		Expect(handler1.EventHandled).To(ContainElement(evt1))
		Expect(handler1.EventHandled).To(ContainElement(evt4))
		Expect(handler2.EventHandled[0]).To(BeIdenticalTo(evt2))
		Expect(handler3.EventHandled).To(ContainElement(evt3))
	})
})
