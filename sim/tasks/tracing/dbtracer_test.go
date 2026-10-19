package tracing

import (
	"os"

	"testing"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// Simple test time teller implementation
type testTimeTeller struct {
	currentTime timing.VTimeInPicoSec
}

func (subject *testTimeTeller) CurrentTime() timing.VTimeInPicoSec {
	return subject.currentTime
}

func (subject *testTimeTeller) SetCurrentTime(time timing.VTimeInPicoSec) {
	subject.currentTime = time
}

func TestDBTracerTasksMilestoneDeduplication(t *testing.T) {
	var (
		timeTeller   *testTimeTeller
		dataRecorder datarecording.DataRecorder
		tracer       *DBTracer
		dbPath       string
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		timeTeller = &testTimeTeller{}
		dbPath = "test_trace_milestone"
		os.Remove(dbPath + ".sqlite3")
		dataRecorder = datarecording.NewDataRecorder(dbPath + ".sqlite3")
		tracer = NewDBTracer(timeTeller, dataRecorder)
	}

	cleanup2 := func(t *testing.T) {
		t.Helper()

		if dataRecorder != nil {
			dataRecorder.Close()
			os.Remove(dbPath + ".sqlite3")
		}
	}

	t.Run("MilestoneAdded with same timestamp", func(t *testing.T) {
		t.Run("should only record the first milestone when multiple milestones occur at the same time", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

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
			require.Len(t, task.Milestones, 1, "Only first milestone should be recorded at same time")
			require.Equal(t, uint64(10), task.Milestones[0].ID)
			require.Equal(t, timing.VTimeInPicoSec(100), task.Milestones[0].Time)
		})

		t.Run("should allow milestones for different tasks at the same time", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

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
			require.Len(t, task1.Milestones, 1)
			require.Len(t, task2.Milestones, 1)
			require.Equal(t, uint64(10), task1.Milestones[0].ID)
			require.Equal(t, uint64(11), task2.Milestones[0].ID)
		})

		t.Run("should allow milestones for same task at different times", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

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
			require.Len(t, task.Milestones, 2)
			require.Equal(t, timing.VTimeInPicoSec(100), task.Milestones[0].Time)
			require.Equal(t, timing.VTimeInPicoSec(200), task.Milestones[1].Time)
		})

		t.Run("should still prevent identical milestones from being recorded twice", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

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
			require.Len(t, task.Milestones, 1)
		})

		t.Run("should keep a repeated (kind, what) at different times", func(t *testing.T) {
			t.Cleanup(func() { cleanup2(t) })
			setup1(t)

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
			require.Len(t, task.Milestones, 2)
			require.Equal(t, timing.VTimeInPicoSec(100), task.Milestones[0].Time)
			require.Equal(t, timing.VTimeInPicoSec(200), task.Milestones[1].Time)
		})
	})
}
