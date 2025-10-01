package tracing

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/sim"
)

// Simple test time teller implementation
type testTimeTeller struct {
	currentTime sim.VTimeInSec
}

func (t *testTimeTeller) CurrentTime() sim.VTimeInSec {
	return t.currentTime
}

func (t *testTimeTeller) SetCurrentTime(time sim.VTimeInSec) {
	t.currentTime = time
}

var _ = Describe("DBTracer Milestone Deduplication", func() {
	var (
		timeTeller   *testTimeTeller
		dataRecorder datarecording.DataRecorder
		tracer       *DBTracer
	)

	BeforeEach(func() {
		timeTeller = &testTimeTeller{}
		dataRecorder = datarecording.NewDataRecorder("/tmp/test_trace_milestone")
		tracer = NewDBTracer(timeTeller, dataRecorder)
	})

	AfterEach(func() {
		if dataRecorder != nil {
			dataRecorder.Close()
			os.Remove("/tmp/test_trace_milestone.sqlite3")
		}
	})

	Context("AddMilestone with same timestamp", func() {
		It("should only record the first milestone when multiple milestones occur at the same time", func() {
			timeTeller.SetCurrentTime(100.0)

			milestone1 := Milestone{
				ID:       "milestone1",
				TaskID:   "task1",
				Kind:     MilestoneKindHardwareResource,
				What:     "resource_acquired",
				Location: "test_location",
			}

			milestone2 := Milestone{
				ID:       "milestone2",
				TaskID:   "task1",
				Kind:     MilestoneKindNetworkTransfer,
				What:     "data_sent",
				Location: "test_location",
			}

			milestone3 := Milestone{
				ID:       "milestone3",
				TaskID:   "task1",
				Kind:     MilestoneKindQueue,
				What:     "queued",
				Location: "test_location",
			}

			tracer.AddMilestone(milestone1)
			tracer.AddMilestone(milestone2) // Same time, different milestone - should be ignored
			tracer.AddMilestone(milestone3) // Same time, different milestone - should be ignored

			task := tracer.tracingTasks["task1"]
			Expect(task.Milestones).To(HaveLen(1), "Only first milestone should be recorded at same time")
			Expect(task.Milestones[0].ID).To(Equal("milestone1"))
			Expect(task.Milestones[0].Time).To(Equal(sim.VTimeInSec(100.0)))
		})

		It("should allow milestones for different tasks at the same time", func() {
			timeTeller.SetCurrentTime(100.0)

			milestone1 := Milestone{
				ID:       "milestone1",
				TaskID:   "task1",
				Kind:     MilestoneKindHardwareResource,
				What:     "resource_acquired",
				Location: "test_location",
			}

			milestone2 := Milestone{
				ID:       "milestone2",
				TaskID:   "task2", // Different task
				Kind:     MilestoneKindNetworkTransfer,
				What:     "data_sent",
				Location: "test_location",
			}

			tracer.AddMilestone(milestone1)
			tracer.AddMilestone(milestone2)

			task1 := tracer.tracingTasks["task1"]
			task2 := tracer.tracingTasks["task2"]
			Expect(task1.Milestones).To(HaveLen(1))
			Expect(task2.Milestones).To(HaveLen(1))
			Expect(task1.Milestones[0].ID).To(Equal("milestone1"))
			Expect(task2.Milestones[0].ID).To(Equal("milestone2"))
		})

		It("should allow milestones for same task at different times", func() {
			timeTeller.SetCurrentTime(100.0)

			milestone1 := Milestone{
				ID:       "milestone1",
				TaskID:   "task1",
				Kind:     MilestoneKindHardwareResource,
				What:     "resource_acquired",
				Location: "test_location",
			}

			tracer.AddMilestone(milestone1)

			timeTeller.SetCurrentTime(200.0)

			milestone2 := Milestone{
				ID:       "milestone2",
				TaskID:   "task1",
				Kind:     MilestoneKindNetworkTransfer,
				What:     "data_sent",
				Location: "test_location",
			}

			tracer.AddMilestone(milestone2)

			task := tracer.tracingTasks["task1"]
			Expect(task.Milestones).To(HaveLen(2))
			Expect(task.Milestones[0].Time).To(Equal(sim.VTimeInSec(100.0)))
			Expect(task.Milestones[1].Time).To(Equal(sim.VTimeInSec(200.0)))
		})

		It("should still prevent identical milestones from being recorded twice", func() {
			timeTeller.SetCurrentTime(100.0)

			milestone := Milestone{
				ID:       "milestone1",
				TaskID:   "task1",
				Kind:     MilestoneKindHardwareResource,
				What:     "resource_acquired",
				Location: "test_location",
			}

			tracer.AddMilestone(milestone)
			tracer.AddMilestone(milestone) // Same milestone again

			task := tracer.tracingTasks["task1"]
			Expect(task.Milestones).To(HaveLen(1))
		})
	})
})