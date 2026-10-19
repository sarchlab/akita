package tracing

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestBackTraceTracer(t *testing.T) {
	var (
		mockCtrl        *gomock.Controller
		mockTaskPrinter *MockTaskPrinter
		tracer          *BackTraceTracer
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		mockCtrl = gomock.NewController(t)
		mockTaskPrinter = NewMockTaskPrinter(mockCtrl)

		tracer = NewBackTraceTracer(mockTaskPrinter)
	}

	t.Run("should trace a single task", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})

		require.Equal(t, 1, len(tracer.tracingTasks))
		require.Equal(t, uint64(0), tracer.tracingTasks[1].ParentID)
	})

	t.Run("should trace two tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})
		tracer.TaskStarted(tasks.Task{ID: 2, ParentID: 1})

		require.Equal(t, 2, len(tracer.tracingTasks))
		require.Equal(t, uint64(0), tracer.tracingTasks[1].ParentID)
		require.Equal(t, uint64(1), tracer.tracingTasks[2].ParentID)
	})

	t.Run("should trace three tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})
		tracer.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		tracer.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		require.Equal(t, 3, len(tracer.tracingTasks))
		require.Equal(t, uint64(0), tracer.tracingTasks[1].ParentID)
		require.Equal(t, uint64(1), tracer.tracingTasks[2].ParentID)
		require.Equal(t, uint64(2), tracer.tracingTasks[3].ParentID)
	})

	t.Run("should end a single task", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})

		tracer.TaskEnded(tasks.Task{ID: 1})

		require.Equal(t, 0, len(tracer.tracingTasks))
	})

	t.Run("should end two tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})
		tracer.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		tracer.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		tracer.TaskEnded(tasks.Task{ID: 3})
		tracer.TaskEnded(tasks.Task{ID: 2})

		require.Equal(t, 1, len(tracer.tracingTasks))
		require.Equal(t, uint64(0), tracer.tracingTasks[1].ParentID)
	})

	t.Run("should print single tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})

		mockTaskPrinter.EXPECT().Print(tasks.Task{ID: 1})

		tracer.DumpBackTrace(tasks.Task{ID: 1})
	})

	t.Run("should print three tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})
		tracer.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		tracer.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		e3 := mockTaskPrinter.EXPECT().
			Print(tasks.Task{ID: 3, ParentID: 2})
		e2 := mockTaskPrinter.EXPECT().
			Print(tasks.Task{ID: 2, ParentID: 1}).
			After(e3)
		mockTaskPrinter.EXPECT().Print(tasks.Task{ID: 1}).
			After(e2)

		tracer.DumpBackTrace(tasks.Task{ID: 3, ParentID: 2})
	})

	t.Run("should print three tasks", func(t *testing.T) {
		setup1(t)

		tracer.TaskStarted(tasks.Task{ID: 1})
		tracer.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		tracer.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		tracer.TaskEnded(tasks.Task{ID: 2})

		mockTaskPrinter.EXPECT().
			Print(tasks.Task{ID: 3, ParentID: 2})

		tracer.DumpBackTrace(tasks.Task{ID: 3, ParentID: 2})
	})
}
