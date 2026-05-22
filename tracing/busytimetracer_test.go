package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/sarchlab/akita/v5/timing"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("BusyTimeTracer", func() {
	var (
		mockCtrl   *gomock.Controller
		timeTeller *MockTimeTeller
		t          *BusyTimeTracer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		timeTeller = NewMockTimeTeller(mockCtrl)

		t = NewBusyTimeTracer(timeTeller, nil)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should track busy time, one task", func() {
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(10))
		t.StartTask(Task{ID: 1})

		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(20))
		t.EndTask(Task{ID: 1})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInSec(10)))
	})

	It("should track busy time, two tasks", func() {
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(10))
		t.StartTask(Task{ID: 1})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(20))
		t.EndTask(Task{ID: 1})

		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(30))
		t.StartTask(Task{ID: 2})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(40))
		t.EndTask(Task{ID: 2})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInSec(20)))
	})

	It("should track busy time, two tasks adjacent", func() {
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(10))
		t.StartTask(Task{ID: 1})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(20))
		t.EndTask(Task{ID: 1})

		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(20))
		t.StartTask(Task{ID: 2})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(30))
		t.EndTask(Task{ID: 2})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInSec(20)))
	})

	It("should track busy time, two tasks overlap", func() {
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(10))
		t.StartTask(Task{ID: 1})

		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(15))
		t.StartTask(Task{ID: 2})

		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(20))
		t.EndTask(Task{ID: 1})

		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(25))
		t.EndTask(Task{ID: 2})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInSec(15)))
	})

	It("should track busy time, four tasks", func() {
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(10))
		t.StartTask(Task{ID: 1})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(11))
		t.StartTask(Task{ID: 2})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(12))
		t.EndTask(Task{ID: 2})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(19))
		t.StartTask(Task{ID: 3})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(20))
		t.EndTask(Task{ID: 1})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(21))
		t.EndTask(Task{ID: 3})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(31))
		t.StartTask(Task{ID: 4})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(32))
		t.EndTask(Task{ID: 4})

		Expect(t.BusyTime()).To(Equal(timing.VTimeInSec(12)))
	})

	It("should be able to terminate all the tasks", func() {
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(10))
		t.StartTask(Task{ID: 1})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(11))
		t.StartTask(Task{ID: 2})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(19))
		t.StartTask(Task{ID: 3})
		timeTeller.EXPECT().CurrentTime().Return(timing.VTimeInSec(21))
		t.EndTask(Task{ID: 3})

		t.TerminateAllTasks(35)

		Expect(t.BusyTime()).To(Equal(timing.VTimeInSec(25)))
	})

	It("measure busy time tracer", func() {
		experiment := gmeasure.NewExperiment("Busy Time Tracer Performance")
		AddReportEntry(experiment.Name, experiment)

		experiment.MeasureDuration("runtime", func() {
			for i := 0; i < 10000; i++ {
				taskID := uint64(i + 1)

				timeTeller.EXPECT().CurrentTime().
					Return(timing.VTimeInSec(i * 2))
				t.StartTask(Task{
					ID: taskID,
				})

				timeTeller.EXPECT().CurrentTime().
					Return(timing.VTimeInSec(i*2 + 1))
				t.EndTask((Task{
					ID:      taskID,
					EndTime: timing.VTimeInSec(i*2 + 1),
				}))
			}

			Expect(t.BusyTime()).To(Equal(timing.VTimeInSec(10000)))
		})
	})
})
