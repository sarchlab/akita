package modeling

import (
	"github.com/sarchlab/akita/v5/timing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type testEngine struct {
	now       timing.VTimeInPicoSec
	scheduled []timing.Event
}

func (e *testEngine) CurrentTime() timing.VTimeInPicoSec {
	return e.now
}

func (e *testEngine) Schedule(event timing.Event) {
	e.scheduled = append(e.scheduled, event)
}

type testTicker struct {
	progress bool
}

func (t *testTicker) Tick() bool {
	return t.progress
}

var _ = Describe("Ticking Component", func() {
	var (
		engine *testEngine
		ticker *testTicker
		tc     *TickingComponent
	)

	BeforeEach(func() {
		engine = &testEngine{now: timing.VTimeInPicoSec(10000)}
		ticker = &testTicker{}
		tc = NewTickingComponent("TC", engine, 1*timing.GHz, ticker)
	})

	It("should start ticking when notified of receiving a request", func() {
		tc.NotifyRecv(nil)

		Expect(engine.scheduled).To(HaveLen(1))
		Expect(engine.scheduled[0].Time()).To(Equal(timing.VTimeInPicoSec(11000)))
	})

	It("should start ticking when notified of a port becoming available",
		func() {
			tc.NotifyPortFree(nil)

			Expect(engine.scheduled).To(HaveLen(1))
			Expect(engine.scheduled[0].Time()).
				To(Equal(timing.VTimeInPicoSec(11000)))
		})

	It("should tick when the ticker make progress in a tick", func() {
		ticker.progress = true

		tc.Handle(MakeTickEvent(tc.Name(), timing.VTimeInPicoSec(10000)))

		Expect(engine.scheduled).To(HaveLen(1))
		Expect(engine.scheduled[0].Time()).To(Equal(timing.VTimeInPicoSec(11000)))
	})

	It("should not tick if there is another tick scheduled in the future",
		func() {
			ticker.progress = true

			tc.Handle(MakeTickEvent(tc.Name(), timing.VTimeInPicoSec(10000)))
			tc.TickNow()

			Expect(engine.scheduled).To(HaveLen(1))
			Expect(engine.scheduled[0].Time()).
				To(Equal(timing.VTimeInPicoSec(11000)))
		})

	It("should stop ticking if no progress is made", func() {
		ticker.progress = false

		tc.Handle(MakeTickEvent(tc.Name(), timing.VTimeInPicoSec(10000)))

		Expect(engine.scheduled).To(BeEmpty())
	})
})
