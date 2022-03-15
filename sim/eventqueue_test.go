package sim

import (
	"math/rand"

	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventQueueImpl", func() {
	var (
		mockCtrl *gomock.Controller
		queue    *EventQueueImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		queue = NewEventQueue()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should pop in order", func() {
		numEvents := 100
		for i := 0; i < numEvents; i++ {
			event := NewMockEvent(mockCtrl)
			event.EXPECT().
				Time().
				Return(VTimeInSec(rand.Float64() / 1e8)).
				AnyTimes()
			queue.Push(event)
		}

		now := VTimeInSec(-1)
		for i := 0; i < numEvents; i++ {
			event := queue.Pop()
			Expect(event.Time() > now).To(BeTrue())
			now = event.Time()
		}
	})
})

var _ = Describe("Insertion Queue", func() {
	var (
		mockCtrl *gomock.Controller
		queue    *InsertionQueue
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		queue = NewInsertionQueue()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should pop in order", func() {
		numEvents := 100
		for i := 0; i < numEvents; i++ {
			event := NewMockEvent(mockCtrl)
			event.EXPECT().
				Time().
				Return(VTimeInSec(rand.Float64() / 1e8)).
				AnyTimes()
			queue.Push(event)
		}

		now := VTimeInSec(-1)
		for i := 0; i < numEvents; i++ {
			event := queue.Pop()
			Expect(event.Time() > now).To(BeTrue())
			now = event.Time()
		}
	})
})
