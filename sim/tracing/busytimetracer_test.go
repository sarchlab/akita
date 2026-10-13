package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var _ = Describe("BusyTimeTracer", func() {
	var (
		t *BusyTimeTracer
	)

	BeforeEach(func() {
		t = NewBusyTimeTracer(nil)
	})

	It("should track busy time, one task", func() {
		t.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		t.TaskEnded(tasks.Task{ID: 1, EndTime: 20})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(10)))
	})

	It("should track busy time, two tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		t.TaskEnded(tasks.Task{ID: 1, EndTime: 20})

		t.TaskStarted(tasks.Task{ID: 2, StartTime: 30})
		t.TaskEnded(tasks.Task{ID: 2, EndTime: 40})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(20)))
	})

	It("should track busy time, two tasks adjacent", func() {
		t.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		t.TaskEnded(tasks.Task{ID: 1, EndTime: 20})

		t.TaskStarted(tasks.Task{ID: 2, StartTime: 20})
		t.TaskEnded(tasks.Task{ID: 2, EndTime: 30})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(20)))
	})

	It("should track busy time, two tasks overlap", func() {
		t.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		t.TaskStarted(tasks.Task{ID: 2, StartTime: 15})
		t.TaskEnded(tasks.Task{ID: 1, EndTime: 20})
		t.TaskEnded(tasks.Task{ID: 2, EndTime: 25})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(15)))
	})

	It("should track busy time, four tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		t.TaskStarted(tasks.Task{ID: 2, StartTime: 11})
		t.TaskEnded(tasks.Task{ID: 2, EndTime: 12})
		t.TaskStarted(tasks.Task{ID: 3, StartTime: 19})
		t.TaskEnded(tasks.Task{ID: 1, EndTime: 20})
		t.TaskEnded(tasks.Task{ID: 3, EndTime: 21})
		t.TaskStarted(tasks.Task{ID: 4, StartTime: 31})
		t.TaskEnded(tasks.Task{ID: 4, EndTime: 32})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(12)))
	})

	It("should be able to terminate all the tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		t.TaskStarted(tasks.Task{ID: 2, StartTime: 11})
		t.TaskStarted(tasks.Task{ID: 3, StartTime: 19})
		t.TaskEnded(tasks.Task{ID: 3, EndTime: 21})

		t.TerminateAllTasks(35)

		Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(25)))
	})

	It("measure busy time tracer", func() {
		experiment := gmeasure.NewExperiment("Busy Time Tracer Performance")
		AddReportEntry(experiment.Name, experiment)

		experiment.MeasureDuration("runtime", func() {
			for i := 0; i < 10000; i++ {
				taskID := uint64(i + 1)

				t.TaskStarted(tasks.Task{ID: taskID, StartTime: timing.VTimeInPicoSec(i * 2)})
				t.TaskEnded(tasks.Task{ID: taskID, EndTime: timing.VTimeInPicoSec(i*2 + 1)})
			}

			Expect(t.BusyTime()).To(Equal(timing.VTimeInPicoSec(10000)))
		})
	})
})
