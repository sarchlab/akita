package tracing

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Simple test time teller implementation
type testTimeTeller struct {
	currentTime timing.VTimeInPicoSec
}

func (t *testTimeTeller) CurrentTime() timing.VTimeInPicoSec {
	return t.currentTime
}

func (t *testTimeTeller) SetCurrentTime(time timing.VTimeInPicoSec) {
	t.currentTime = time
}

var _ = Describe("DBTracer tasks.Milestone Deduplication", func() {
	var (
		timeTeller   *testTimeTeller
		dataRecorder datarecording.DataRecorder
		tracer       *DBTracer
		dbPath       string
	)

	BeforeEach(func() {
		timeTeller = &testTimeTeller{}
		dbPath = "test_trace_milestone"
		os.Remove(dbPath + ".sqlite3")
		dataRecorder = datarecording.NewDataRecorder(dbPath)
		tracer = NewDBTracer(timeTeller, dataRecorder)
	})

	AfterEach(func() {
		if dataRecorder != nil {
			dataRecorder.Close()
			os.Remove(dbPath + ".sqlite3")
		}
	})

	Context("MilestoneAdded with same timestamp", func() {
		It("should only record the first milestone when multiple milestones occur at the same time", func() {
			tracer.StartTracing()

			milestone1 := tasks.Milestone{
				ID:     uint64(10),
				TaskID: uint64(1),
				Time:   100,
				Kind:   tasks.MilestoneKindHardwareResource,
				What:   "resource_acquired",
			}

			milestone2 := tasks.Milestone{
				ID:     uint64(11),
				TaskID: uint64(1),
				Time:   100,
				Kind:   tasks.MilestoneKindNetworkTransfer,
				What:   "data_sent",
			}

			milestone3 := tasks.Milestone{
				ID:     uint64(12),
				TaskID: uint64(1),
				Time:   100,
				Kind:   tasks.MilestoneKindQueue,
				What:   "queued",
			}

			tracer.MilestoneAdded(milestone1)
			tracer.MilestoneAdded(milestone2) // Same time, different milestone - should be ignored
			tracer.MilestoneAdded(milestone3) // Same time, different milestone - should be ignored

			task := tracer.tracingTasks[uint64(1)]
			Expect(task.Milestones).To(HaveLen(1), "Only first milestone should be recorded at same time")
			Expect(task.Milestones[0].ID).To(Equal(uint64(10)))
			Expect(task.Milestones[0].Time).To(Equal(timing.VTimeInPicoSec(100)))
		})

		It("should allow milestones for different tasks at the same time", func() {
			tracer.StartTracing()

			milestone1 := tasks.Milestone{
				ID:     uint64(10),
				TaskID: uint64(1),
				Time:   100,
				Kind:   tasks.MilestoneKindHardwareResource,
				What:   "resource_acquired",
			}

			milestone2 := tasks.Milestone{
				ID:     uint64(11),
				TaskID: uint64(2), // Different task
				Time:   100,
				Kind:   tasks.MilestoneKindNetworkTransfer,
				What:   "data_sent",
			}

			tracer.MilestoneAdded(milestone1)
			tracer.MilestoneAdded(milestone2)

			task1 := tracer.tracingTasks[uint64(1)]
			task2 := tracer.tracingTasks[uint64(2)]
			Expect(task1.Milestones).To(HaveLen(1))
			Expect(task2.Milestones).To(HaveLen(1))
			Expect(task1.Milestones[0].ID).To(Equal(uint64(10)))
			Expect(task2.Milestones[0].ID).To(Equal(uint64(11)))
		})

		It("should allow milestones for same task at different times", func() {
			tracer.StartTracing()

			milestone1 := tasks.Milestone{
				ID:     uint64(10),
				TaskID: uint64(1),
				Time:   100,
				Kind:   tasks.MilestoneKindHardwareResource,
				What:   "resource_acquired",
			}

			tracer.MilestoneAdded(milestone1)

			milestone2 := tasks.Milestone{
				ID:     uint64(11),
				TaskID: uint64(1),
				Time:   200,
				Kind:   tasks.MilestoneKindNetworkTransfer,
				What:   "data_sent",
			}

			tracer.MilestoneAdded(milestone2)

			task := tracer.tracingTasks[uint64(1)]
			Expect(task.Milestones).To(HaveLen(2))
			Expect(task.Milestones[0].Time).To(Equal(timing.VTimeInPicoSec(100)))
			Expect(task.Milestones[1].Time).To(Equal(timing.VTimeInPicoSec(200)))
		})

		It("should still prevent identical milestones from being recorded twice", func() {
			tracer.StartTracing()

			milestone := tasks.Milestone{
				ID:     uint64(10),
				TaskID: uint64(1),
				Time:   100,
				Kind:   tasks.MilestoneKindHardwareResource,
				What:   "resource_acquired",
			}

			tracer.MilestoneAdded(milestone)
			tracer.MilestoneAdded(milestone) // Same milestone again

			task := tracer.tracingTasks[uint64(1)]
			Expect(task.Milestones).To(HaveLen(1))
		})

		It("should keep a repeated (kind, what) at different times", func() {
			tracer.StartTracing()

			m1 := tasks.Milestone{
				ID:     uint64(10),
				TaskID: uint64(1),
				Time:   100,
				Kind:   tasks.MilestoneKindHardwareResource,
				What:   "VALU",
			}
			m2 := tasks.Milestone{
				ID:     uint64(11),
				TaskID: uint64(1),
				Time:   200,
				Kind:   tasks.MilestoneKindHardwareResource,
				What:   "VALU",
			}

			tracer.MilestoneAdded(m1)
			// Same (kind, what) but a later time: a long-lived task recurs through
			// the same reason, so this must be kept, not collapsed to the first.
			tracer.MilestoneAdded(m2)

			task := tracer.tracingTasks[uint64(1)]
			Expect(task.Milestones).To(HaveLen(2))
			Expect(task.Milestones[0].Time).To(Equal(timing.VTimeInPicoSec(100)))
			Expect(task.Milestones[1].Time).To(Equal(timing.VTimeInPicoSec(200)))
		})
	})
})
