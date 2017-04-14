package core_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("ParallelEngine", func() {
	var (
		engine *core.ParallelEngine
	)

	BeforeEach(func() {
		engine = core.NewParallelEngine()
	})

	It("should schedule events", func() {
		handler1 := NewMockHandler()
		handler2 := NewMockHandler()
		handler3 := NewMockHandler()
		evt1 := NewMockEvent()
		evt2 := NewMockEvent()
		evt3 := NewMockEvent()
		evt4 := NewMockEvent()

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

		handler1.HandleFunc = func(e core.Event) {
		}
		handler2.HandleFunc = func(e core.Event) {
			engine.Schedule(evt3)
			engine.Schedule(evt4)
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
