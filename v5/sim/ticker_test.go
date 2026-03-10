package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Ticking Component", func() {
	var (
		mockCtrl *gomock.Controller
		engine   *MockEngine
		ticker   *MockTicker
		tc       *TickingComponent
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		ticker = NewMockTicker(mockCtrl)
		tc = NewTickingComponent("TC", engine, 1, ticker)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should start ticking when notified of receiving a request", func() {
		engine.EXPECT().Schedule(gomock.Any()).
			Do(func(e TickEvent) {
				Expect(e.Time()).To(Equal(VTimeInSec(11)))
			})
		engine.EXPECT().CurrentTime().Return(VTimeInSec(10))
		tc.NotifyRecv(nil)
	})

	It("should start ticking when notified of a port becoming available",
		func() {
			engine.EXPECT().Schedule(gomock.Any()).
				Do(func(e TickEvent) {
					Expect(e.Time()).To(Equal(VTimeInSec(11)))
				})
			engine.EXPECT().CurrentTime().Return(VTimeInSec(10))
			tc.NotifyPortFree(nil)
		})

	It("should tick when the ticker make progress in a tick", func() {
		engine.EXPECT().Schedule(gomock.Any()).
			Do(func(e TickEvent) {
				Expect(e.Time()).To(Equal(VTimeInSec(11)))
			})
		ticker.EXPECT().Tick().Return(true)
		engine.EXPECT().CurrentTime().Return(VTimeInSec(10))
		tc.Handle(MakeTickEvent(tc, VTimeInSec(10)))
	})

	It("should not tick if there is another tick scheduled in the future",
		func() {
			engine.EXPECT().Schedule(gomock.Any()).
				Do(func(e TickEvent) {
					Expect(e.Time()).To(Equal(VTimeInSec(11)))
				})

			ticker.EXPECT().Tick().Return(true)
			engine.EXPECT().CurrentTime().Return(VTimeInSec(10))
			tc.Handle(MakeTickEvent(tc, VTimeInSec(10)))

			engine.EXPECT().CurrentTime().Return(VTimeInSec(10))
			tc.TickNow()
		})

	It("should stop ticking if no progress is made", func() {
		ticker.EXPECT().Tick().Return(false)
		tc.Handle(MakeTickEvent(tc, VTimeInSec(10)))
		engine.EXPECT().Schedule(gomock.Any()).Times(0)
	})

})

var _ = Describe("TickScheduler Reset", func() {
	var (
		mockCtrl  *gomock.Controller
		engine    *MockEngine
		scheduler *TickScheduler
		handler   *MockHandler
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		handler = NewMockHandler(mockCtrl)
		scheduler = NewTickScheduler(handler, engine, 1)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should reset nextTickTime to -1", func() {
		// Simulate a scheduled tick so nextTickTime advances
		engine.EXPECT().CurrentTime().Return(VTimeInSec(10))
		engine.EXPECT().Schedule(gomock.Any())
		scheduler.TickLater()
		Expect(scheduler.nextTickTime).To(Equal(VTimeInSec(11)))

		// After reset, nextTickTime should be -1
		scheduler.Reset()
		Expect(scheduler.nextTickTime).To(Equal(VTimeInSec(-1)))
	})

	It("should allow TickLater to schedule again after reset", func() {
		// First TickLater
		engine.EXPECT().CurrentTime().Return(VTimeInSec(10))
		engine.EXPECT().Schedule(gomock.Any())
		scheduler.TickLater()

		// Reset
		scheduler.Reset()

		// TickLater should work again even at an earlier time
		engine.EXPECT().CurrentTime().Return(VTimeInSec(5))
		engine.EXPECT().Schedule(gomock.Any())
		scheduler.TickLater()
	})
})
