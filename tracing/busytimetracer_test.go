package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("BusyTimeTracer", func() {
	var (
		t *BusyTimeTracer
	)

	BeforeEach(func() {
		t = NewBusyTimeTracer(nil)
	})

	It("should track busy time, one task", func() {
		t.StartTask(TaskStart{ID: 1, Time: 10})
		t.EndTask(TaskEnd{ID: 1, Time: 20})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(10)))
	})

	It("should track busy time, two tasks", func() {
		t.StartTask(TaskStart{ID: 1, Time: 10})
		t.EndTask(TaskEnd{ID: 1, Time: 20})

		t.StartTask(TaskStart{ID: 2, Time: 30})
		t.EndTask(TaskEnd{ID: 2, Time: 40})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(20)))
	})

	It("should track busy time, two tasks adjacent", func() {
		t.StartTask(TaskStart{ID: 1, Time: 10})
		t.EndTask(TaskEnd{ID: 1, Time: 20})

		t.StartTask(TaskStart{ID: 2, Time: 20})
		t.EndTask(TaskEnd{ID: 2, Time: 30})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(20)))
	})

	It("should track busy time, two tasks overlap", func() {
		t.StartTask(TaskStart{ID: 1, Time: 10})
		t.StartTask(TaskStart{ID: 2, Time: 15})
		t.EndTask(TaskEnd{ID: 1, Time: 20})
		t.EndTask(TaskEnd{ID: 2, Time: 25})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(15)))
	})

	It("should track busy time, four tasks", func() {
		t.StartTask(TaskStart{ID: 1, Time: 10})
		t.StartTask(TaskStart{ID: 2, Time: 11})
		t.EndTask(TaskEnd{ID: 2, Time: 12})
		t.StartTask(TaskStart{ID: 3, Time: 19})
		t.EndTask(TaskEnd{ID: 1, Time: 20})
		t.EndTask(TaskEnd{ID: 3, Time: 21})
		t.StartTask(TaskStart{ID: 4, Time: 31})
		t.EndTask(TaskEnd{ID: 4, Time: 32})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(12)))
	})

	It("should be able to terminate all the tasks", func() {
		t.StartTask(TaskStart{ID: 1, Time: 10})
		t.StartTask(TaskStart{ID: 2, Time: 11})
		t.StartTask(TaskStart{ID: 3, Time: 19})
		t.EndTask(TaskEnd{ID: 3, Time: 21})

		t.TerminateAllTasks(35)

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(25)))
	})

	It("measure busy time tracer", func() {
		experiment := gmeasure.NewExperiment("Busy Time Tracer Performance")
		AddReportEntry(experiment.Name, experiment)

		experiment.MeasureDuration("runtime", func() {
			for i := 0; i < 10000; i++ {
				taskID := uint64(i + 1)

				t.StartTask(TaskStart{ID: taskID, Time: timing.VTimeInPicoSec(i * 2)})
				t.EndTask(TaskEnd{ID: taskID, Time: timing.VTimeInPicoSec(i*2 + 1)})
			}

			Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(10000)))
		})
	})
})
