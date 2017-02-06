package event_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/core/event"
	"gitlab.com/yaotsu/core/event/mock_event"
)

var _ = Describe("Engine", func() {
	var (
		mockCtrl *gomock.Controller
		engine   *event.Engine
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = event.NewEngine()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should start with no event", func() {
		Expect(engine.HasMoreEvent()).To(Equal(false))
	})

	It("should schedule event", func() {
		e := mock_event.NewMockEvent(mockCtrl)

		e.EXPECT().SetTime(event.VTimeInSec(10))
		e.EXPECT().Time().Return(event.VTimeInSec(10)).AnyTimes()
		engine.Schedule(e, event.VTimeInSec(10))
		Expect(engine.HasMoreEvent()).To(Equal(true))

		e.EXPECT().Happen()
		engine.Run()

		Expect(engine.Now()).To(Equal(event.VTimeInSec(10.0)))
	})

	It("should execute in time order", func() {

		e1 := mock_event.NewMockEvent(mockCtrl)
		e2 := mock_event.NewMockEvent(mockCtrl)
		e3 := mock_event.NewMockEvent(mockCtrl)
		e4 := mock_event.NewMockEvent(mockCtrl)

		e1.EXPECT().SetTime(event.VTimeInSec(0))
		e1.EXPECT().Time().Return(event.VTimeInSec(0)).AnyTimes()
		engine.Schedule(e1, 0)

		e2.EXPECT().SetTime(event.VTimeInSec(10))
		e2.EXPECT().Time().Return(event.VTimeInSec(10)).AnyTimes()
		engine.Schedule(e2, 10)

		e3.EXPECT().SetTime(event.VTimeInSec(10))
		e3.EXPECT().Time().Return(event.VTimeInSec(10)).AnyTimes()
		engine.Schedule(e3, 10)

		e1.EXPECT().Happen()
		engine.Run()
		Expect(engine.Now()).To(Equal(event.VTimeInSec(0.0)))

		e2.EXPECT().Happen()
		e3.EXPECT().Happen()
		engine.Run()
		Expect(engine.Now()).To(Equal(event.VTimeInSec(10.0)))

		engine.Run()
		Expect(engine.Now()).To(Equal(event.VTimeInSec(10.0)))

		e4.EXPECT().SetTime(event.VTimeInSec(110))
		e4.EXPECT().Time().Return(event.VTimeInSec(110)).AnyTimes()
		engine.Schedule(e4, 100)
		e4.EXPECT().Happen()
		engine.Run()
		Expect(engine.Now()).To(Equal(event.VTimeInSec(110.0)))
	})
})
