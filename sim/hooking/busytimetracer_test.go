package hooking

import (
	"fmt"

	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
)

type stubTimeTeller struct {
	now float64
}

func (t *stubTimeTeller) Now() float64 {
	return t.now
}

var _ = Describe("BusyTimeTracer", func() {
	var (
		mockCtrl   *gomock.Controller
		timeTeller *stubTimeTeller
		t          *BusyTimeTracer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		timeTeller = &stubTimeTeller{}

		t = NewBusyTimeTracer(timeTeller, nil)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should track busy time, one task", func() {
		timeTeller.now = 1
		t.StartTask(TaskStart{ID: "1"})

		timeTeller.now = 2
		t.EndTask(TaskEnd{ID: "1"})

		Expect(t.BusyTime()).To(Equal(1.0))
	})

	It("should track busy time, two tasks", func() {
		timeTeller.now = 1
		t.StartTask(TaskStart{ID: "1"})
		timeTeller.now = 2
		t.EndTask(TaskEnd{ID: "1"})

		timeTeller.now = 3
		t.StartTask(TaskStart{ID: "2"})
		timeTeller.now = 4
		t.EndTask(TaskEnd{ID: "2"})

		Expect(t.BusyTime()).To(Equal(2.0))
	})

	It("should track busy time, two tasks adjacent", func() {
		timeTeller.now = 1
		t.StartTask(TaskStart{ID: "1"})
		timeTeller.now = 2
		t.EndTask(TaskEnd{ID: "1"})

		timeTeller.now = 3
		t.StartTask(TaskStart{ID: "2"})
		timeTeller.now = 4
		t.EndTask(TaskEnd{ID: "2"})

		Expect(t.BusyTime()).To(Equal(2.0))
	})

	It("should track busy time, two tasks overlap", func() {
		timeTeller.now = 1
		t.StartTask(TaskStart{ID: "1"})

		timeTeller.now = 1.5
		t.StartTask(TaskStart{ID: "2"})

		timeTeller.now = 2
		t.EndTask(TaskEnd{ID: "1"})

		timeTeller.now = 2.5
		t.EndTask(TaskEnd{ID: "2"})

		Expect(t.BusyTime()).To(Equal(1.5))
	})

	It("should track busy time, four tasks", func() {
		timeTeller.now = 1
		t.StartTask(TaskStart{ID: "1"})
		timeTeller.now = 1.1
		t.StartTask(TaskStart{ID: "2"})
		timeTeller.now = 1.2
		t.EndTask(TaskEnd{ID: "2"})
		timeTeller.now = 1.9
		t.StartTask(TaskStart{ID: "3"})
		timeTeller.now = 2
		t.EndTask(TaskEnd{ID: "1"})
		timeTeller.now = 2.1
		t.EndTask(TaskEnd{ID: "3"})
		timeTeller.now = 3.1
		t.StartTask(TaskStart{ID: "4"})
		timeTeller.now = 3.2
		t.EndTask(TaskEnd{ID: "4"})

		Expect(t.BusyTime()).To(BeNumerically("~", 1.2))
	})

	It("should be able to terminate all the tasks", func() {
		timeTeller.now = 1
		t.StartTask(TaskStart{ID: "1"})
		timeTeller.now = 1.1
		t.StartTask(TaskStart{ID: "2"})
		timeTeller.now = 1.9
		t.StartTask(TaskStart{ID: "3"})
		timeTeller.now = 2.1
		t.EndTask(TaskEnd{ID: "3"})

		timeTeller.now = 3.5
		t.TerminateAllTasks()

		Expect(t.BusyTime()).To(BeNumerically("~", 2.5, 0.01))
	})

	It("measure busy time tracer", func() {
		experiment := gmeasure.NewExperiment("Busy Time Tracer Performance")
		AddReportEntry(experiment.Name, experiment)

		experiment.MeasureDuration("runtime", func() {
			for i := 0; i < 10000; i++ {
				taskID := fmt.Sprintf("%d", i)

				timeTeller.now = float64(i * 2)
				t.StartTask(TaskStart{
					ID: taskID,
				})

				timeTeller.now = float64(i*2 + 1)
				t.EndTask(TaskEnd{
					ID: taskID,
				})
			}

			Expect(t.BusyTime()).To(BeNumerically("~", 10000, 0.01))
		})
	})
})
