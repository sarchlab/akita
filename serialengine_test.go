package akita

import (
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SerialEngine", func() {
	var (
		engine *SerialEngine
	)

	BeforeEach(func() {
		engine = NewSerialEngine()
	})

	It("should schedule events", func() {
		handler1 := newMockHandler()
		handler2 := newMockHandler()
		evt1 := newMockEvent()
		evt2 := newMockEvent()
		evt3 := newMockEvent()
		evt4 := newMockEvent()

		// Four events to be scheduled. Evt1 and Evt2 are directly scheduled,
		// while evt2 schedules evt3 and evt4. They should be executed
		// in the global time order
		evt1.SetTime(4.0)
		evt1.SetHandler(handler1)
		evt2.SetTime(2.0)
		evt2.SetHandler(handler2)
		evt3.SetTime(3.0)
		evt3.SetHandler(handler1)
		evt4.SetTime(5.0)
		evt4.SetHandler(handler1)

		handler1.HandleFunc = func(e Event) {
		}
		handler2.HandleFunc = func(e Event) {
			engine.Schedule(evt3)
			engine.Schedule(evt4)
		}

		engine.Schedule(evt1)
		engine.Schedule(evt2)

		_ = engine.Run()

		Expect(handler1.EventHandled[0]).To(BeIdenticalTo(evt3))
		Expect(handler1.EventHandled[1]).To(BeIdenticalTo(evt1))
		Expect(handler1.EventHandled[2]).To(BeIdenticalTo(evt4))
		Expect(handler2.EventHandled[0]).To(BeIdenticalTo(evt2))
	})

	Measure("Event triggering speed", func(b Benchmarker) {
		handler := newMockHandler()
		handler.HandleFunc = func(e Event) {}

		for i := 0; i < 100000; i++ {
			evt := newMockEvent()
			time := VTimeInSec(float64(rand.Uint64()%100) * 0.01)
			evt.SetTime(time)
			evt.SetHandler(handler)
			engine.Schedule(evt)
		}

		b.Time("runtime", func() { _ = engine.Run() })
	}, 10)
})
