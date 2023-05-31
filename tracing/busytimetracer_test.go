package tracing

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"

	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
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
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1))
		t.StartTask(Task{ID: "1"})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2))
		t.EndTask(Task{ID: "1"})

		Expect(t.BusyTime()).To(Equal(sim.VTimeInSec(1.0)))
	})

	It("should track busy time, two tasks", func() {
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1))
		t.StartTask(Task{ID: "1"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2))
		t.EndTask(Task{ID: "1"})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(3))
		t.StartTask(Task{ID: "2"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(4))
		t.EndTask(Task{ID: "2"})

		Expect(t.BusyTime()).To(Equal(sim.VTimeInSec(2.0)))
	})

	It("should track busy time, two tasks adjacent", func() {
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1))
		t.StartTask(Task{ID: "1"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2))
		t.EndTask(Task{ID: "1"})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2))
		t.StartTask(Task{ID: "2"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(3))
		t.EndTask(Task{ID: "2"})

		Expect(t.BusyTime()).To(Equal(sim.VTimeInSec(2.0)))
	})

	It("should track busy time, two tasks overlap", func() {
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1))
		t.StartTask(Task{ID: "1"})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.5))
		t.StartTask(Task{ID: "2"})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2))
		t.EndTask(Task{ID: "1"})

		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2.5))
		t.EndTask(Task{ID: "2"})

		Expect(t.BusyTime()).To(Equal(sim.VTimeInSec(1.5)))
	})

	It("should track busy time, four tasks", func() {
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1))
		t.StartTask(Task{ID: "1"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.1))
		t.StartTask(Task{ID: "2"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.2))
		t.EndTask(Task{ID: "2"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.9))
		t.StartTask(Task{ID: "3"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2))
		t.EndTask(Task{ID: "1"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2.1))
		t.EndTask(Task{ID: "3"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(3.1))
		t.StartTask(Task{ID: "4"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(3.2))
		t.EndTask(Task{ID: "4"})

		Expect(t.BusyTime()).To(BeNumerically("~", 1.2))
	})

	It("should be able to terminate all the tasks", func() {
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1))
		t.StartTask(Task{ID: "1"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.1))
		t.StartTask(Task{ID: "2"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(1.9))
		t.StartTask(Task{ID: "3"})
		timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(2.1))
		t.EndTask(Task{ID: "3"})

		t.TerminateAllTasks(3.5)

		Expect(t.BusyTime()).To(BeNumerically("~", 2.5, 0.01))
	})

	It("measure busy time tracer", func() {
		experiment := gmeasure.NewExperiment("Busy Time Tracer Performance")
		AddReportEntry(experiment.Name, experiment)

		experiment.MeasureDuration("runtime", func() {
			for i := 0; i < 10000; i++ {
				taskID := fmt.Sprintf("%d", i)

				timeTeller.EXPECT().CurrentTime().
					Return(sim.VTimeInSec(i * 2))
				t.StartTask(Task{
					ID: taskID,
				})

				timeTeller.EXPECT().CurrentTime().
					Return(sim.VTimeInSec(i*2 + 1))
				t.EndTask((Task{
					ID:      taskID,
					EndTime: sim.VTimeInSec(i*2 + 1),
				}))
			}

			Expect(t.BusyTime()).To(BeNumerically("~", 10000, 0.01))
		})
	})
})
