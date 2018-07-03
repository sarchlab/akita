package core

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"math/rand"
)

var _ = Describe("EventQueueImpl", func(){

	var (
		queue *EventQueueImpl
	)

	BeforeEach(func() {
		queue = NewEventQueue()
	})

	It("should pop in order", func() {
		numEvents := 100
		for i := 0; i < numEvents; i++ {
			event := newMockEvent()
			event.SetTime(VTimeInSec(rand.Float64()/1e8))
			queue.Push(event)
		}

		now := VTimeInSec(-1)
		for i := 0; i < numEvents; i++ {
			event := queue.Pop()
			Expect(event.Time() > now).To(BeTrue())
			now = event.Time()
		}
	})

	It("should pop in order", func() {
		event1 := newMockEvent()
		event1.SetTime(0.0000002620)
		queue.Push(event1)

		numEvents := 100
		for i := 0 ; i < numEvents; i++ {
			event2 := newMockEvent()
			event2.SetTime(0.0000002610)
			queue.Push(event2)
		}

		for i := 0 ; i < numEvents; i++ {
			Expect(queue.Pop().Time()).To(Equal(VTimeInSec(0.0000002610)))
		}

		event3 := newMockEvent()
		event3.SetTime(0.0000002610)
		queue.Push(event3)

		Expect(queue.Pop()).To(BeIdenticalTo(event3))
		Expect(queue.Pop()).To(BeIdenticalTo(event1))

	})

})

var _ = Describe("Insertion Queue", func(){

	var (
		queue *InsertionQueue
	)

	BeforeEach(func() {
		queue = NewInsertionQueue()
	})

	It("should pop in order", func() {
		numEvents := 100
		for i := 0; i < numEvents; i++ {
			event := newMockEvent()
			event.SetTime(VTimeInSec(rand.Float64()/1e8))
			queue.Push(event)
		}

		now := VTimeInSec(-1)
		for i := 0; i < numEvents; i++ {
			event := queue.Pop()
			Expect(event.Time() > now).To(BeTrue())
			now = event.Time()
		}
	})

	It("should pop in order", func() {
		event1 := newMockEvent()
		event1.SetTime(0.0000002620)
		queue.Push(event1)

		numEvents := 100
		for i := 0 ; i < numEvents; i++ {
			event2 := newMockEvent()
			event2.SetTime(0.0000002610)
			queue.Push(event2)
		}

		for i := 0 ; i < numEvents; i++ {
			Expect(queue.Pop().Time()).To(Equal(VTimeInSec(0.0000002610)))
		}

		event3 := newMockEvent()
		event3.SetTime(0.0000002610)
		queue.Push(event3)

		Expect(queue.Pop()).To(BeIdenticalTo(event3))
		Expect(queue.Pop()).To(BeIdenticalTo(event1))

	})

})