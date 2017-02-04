package event_test

import (
	"gitlab.com/yaotsu/core/event"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CycleScheduler", func() {
	engine := event.NewEngine()
	scheduler := event.NewCycleScheduler(engine)
	scheduler.SetFrequecy(1e9) // 1 GHz

	It("should return correct frequency", func() {
		Expect(scheduler.Frequency()).To(BeNumerically("~", 1e9, 1))
	})

	It("should schedule event at the right time", func() {
		engine.Reset()

		event := new(event.BasicEvent)
		scheduler.Schedule(event, 1)

		engine.Run()

		Expect(engine.Now()).To(BeNumerically("~", 1e-9, 1e-18))
	})

	It("should retry", func() {
		engine.Reset()

		event := new(event.BasicEvent)
		scheduler.Schedule(event, 2)

		engine.Run()
		scheduler.Retry(event)
		engine.Run()

		Expect(engine.Now()).To(BeNumerically("~", 3e-9, 1e-18))
	})

})
