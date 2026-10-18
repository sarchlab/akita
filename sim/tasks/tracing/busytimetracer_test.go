package tracing

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestBusyTimeTracer(t *testing.T) {
	var (
		tracer *BusyTimeTracer
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		tracer = NewBusyTimeTracer(nil)
	}

	t.Run("should track busy time, one task", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		tracer.TaskEnded(tasks.Task{ID: 1, StartTime: 10, EndTime: 20})

		require.Equal(t, timing.VTimeInPicoSec(10), tracer.BusyTime())
	})

	t.Run("should track busy time, two tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		tracer.TaskEnded(tasks.Task{ID: 1, StartTime: 10, EndTime: 20})

		tracer.TaskStarted(tasks.Task{ID: 2, StartTime: 30})
		tracer.TaskEnded(tasks.Task{ID: 2, StartTime: 30, EndTime: 40})

		require.Equal(t, timing.VTimeInPicoSec(20), tracer.BusyTime())
	})

	t.Run("should track busy time, two tasks adjacent", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		tracer.TaskEnded(tasks.Task{ID: 1, StartTime: 10, EndTime: 20})

		tracer.TaskStarted(tasks.Task{ID: 2, StartTime: 20})
		tracer.TaskEnded(tasks.Task{ID: 2, StartTime: 20, EndTime: 30})

		require.Equal(t, timing.VTimeInPicoSec(20), tracer.BusyTime())
	})

	t.Run("should track busy time, two tasks overlap", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		tracer.TaskStarted(tasks.Task{ID: 2, StartTime: 15})
		tracer.TaskEnded(tasks.Task{ID: 1, StartTime: 10, EndTime: 20})
		tracer.TaskEnded(tasks.Task{ID: 2, StartTime: 15, EndTime: 25})

		require.Equal(t, timing.VTimeInPicoSec(15), tracer.BusyTime())
	})

	t.Run("should track busy time, four tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1, StartTime: 10})
		tracer.TaskStarted(tasks.Task{ID: 2, StartTime: 11})
		tracer.TaskEnded(tasks.Task{ID: 2, StartTime: 11, EndTime: 12})
		tracer.TaskStarted(tasks.Task{ID: 3, StartTime: 19})
		tracer.TaskEnded(tasks.Task{ID: 1, StartTime: 10, EndTime: 20})
		tracer.TaskEnded(tasks.Task{ID: 3, StartTime: 19, EndTime: 21})
		tracer.TaskStarted(tasks.Task{ID: 4, StartTime: 31})
		tracer.TaskEnded(tasks.Task{ID: 4, StartTime: 31, EndTime: 32})

		require.Equal(t, timing.VTimeInPicoSec(12), tracer.BusyTime())
	})

	t.Run("merges late and out-of-order completions across previously counted gaps", func(t *testing.T) {
		setup1(t)

		tracer.TaskEnded(tasks.Task{StartTime: 20, EndTime: 30})
		tracer.TaskEnded(tasks.Task{StartTime: 40, EndTime: 50})
		require.Equal(t, timing.VTimeInPicoSec(20), tracer.BusyTime())
		tracer.TaskEnded(tasks.Task{StartTime: 10, EndTime: 45})
		require.Equal(t, timing.VTimeInPicoSec(40), tracer.BusyTime())
		tracer.TaskEnded(tasks.Task{StartTime: 15, EndTime: 25})
		tracer.TaskEnded(tasks.Task{StartTime: 50, EndTime: 60})
		require.Equal(t, timing.VTimeInPicoSec(50), tracer.BusyTime())
	})
	t.Run("filters the complete end record and ignores unfinished tasks", func(t *testing.T) {
		setup1(t)

		tracer = NewBusyTimeTracer(func(task tasks.Task) bool { return task.Kind == "selected" })
		tracer.TaskStarted(tasks.Task{ID: 1, Kind: "selected", StartTime: 0})
		tracer.TaskEnded(tasks.Task{ID: 2, Kind: "other", StartTime: 0, EndTime: 100})
		tracer.TaskEnded(tasks.Task{ID: 3, Kind: "selected", StartTime: 10, EndTime: 30})
		require.Equal(t, timing.VTimeInPicoSec(20), tracer.BusyTime())
	})

	t.Run("measure busy time tracer", func(t *testing.T) {
		setup1(t)

		for i := 0; i < 10000; i++ {
			taskID := uint64(i + 1)

			tracer.TaskStarted(tasks.Task{ID: taskID, StartTime: timing.VTimeInPicoSec(i * 2)})
			tracer.TaskEnded(tasks.Task{ID: taskID, StartTime: timing.VTimeInPicoSec(i * 2),
				EndTime: timing.VTimeInPicoSec(i*2 + 1)})
		}

		require.Equal(t, timing.VTimeInPicoSec(10000), tracer.BusyTime())
	})
}

// BenchmarkBusyTimeTracer measures the same 10,000-task workload as the
// correctness subtest above, with a fresh tracer for every iteration.
func BenchmarkBusyTimeTracer(b *testing.B) {
	for b.Loop() {
		tracer := NewBusyTimeTracer(nil)
		for i := 0; i < 10000; i++ {
			taskID := uint64(i + 1)
			tracer.TaskStarted(tasks.Task{ID: taskID, StartTime: timing.VTimeInPicoSec(i * 2)})
			tracer.TaskEnded(tasks.Task{ID: taskID, StartTime: timing.VTimeInPicoSec(i * 2),
				EndTime: timing.VTimeInPicoSec(i*2 + 1)})
		}
		if got := tracer.BusyTime(); got != 10000 {
			b.Fatalf("busy time = %v, want 10000", got)
		}
	}
}
