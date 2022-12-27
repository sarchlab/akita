package sim

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		tc.NotifyRecv(10, nil)
	})

	It("should start ticking when notified of a port becoming available", func() {
		engine.EXPECT().Schedule(gomock.Any()).
			Do(func(e TickEvent) {
				Expect(e.Time()).To(Equal(VTimeInSec(11)))
			})
		tc.NotifyPortFree(10, nil)
	})

	It("should tick when the ticker make progress in a tick", func() {
		engine.EXPECT().Schedule(gomock.Any()).
			Do(func(e TickEvent) {
				Expect(e.Time()).To(Equal(VTimeInSec(11)))
			})
		ticker.EXPECT().Tick(VTimeInSec(10)).Return(true)
		tc.Handle(MakeTickEvent(10, tc))
	})

	It("should not tick if there is another tick scheduled in the future", func() {
		engine.EXPECT().Schedule(gomock.Any()).
			Do(func(e TickEvent) {
				Expect(e.Time()).To(Equal(VTimeInSec(11)))
			})

		ticker.EXPECT().Tick(VTimeInSec(10)).Return(true)
		tc.Handle(MakeTickEvent(10, tc))

		ticker.EXPECT().Tick(VTimeInSec(10)).Return(true)
		tc.Handle(MakeTickEvent(10, tc))
	})

	It("should stop ticking if no progress is made", func() {
		ticker.EXPECT().Tick(VTimeInSec(10)).Return(false)
		tc.Handle(MakeTickEvent(10, tc))
	})

})
