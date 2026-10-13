package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/tasks"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("BackTraceTracer", func() {
	var (
		mockCtrl        *gomock.Controller
		mockTaskPrinter *MockTaskPrinter
		t               *BackTraceTracer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockTaskPrinter = NewMockTaskPrinter(mockCtrl)

		t = NewBackTraceTracer(mockTaskPrinter)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should trace a single task", func() {
		t.TaskStarted(tasks.Task{ID: 1})

		Expect(len(t.tracingTasks)).To(Equal(1))
		Expect(t.tracingTasks[1].ParentID).To(Equal(uint64(0)))
	})

	It("should trace two tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1})
		t.TaskStarted(tasks.Task{ID: 2, ParentID: 1})

		Expect(len(t.tracingTasks)).To(Equal(2))
		Expect(t.tracingTasks[1].ParentID).To(Equal(uint64(0)))
		Expect(t.tracingTasks[2].ParentID).To(Equal(uint64(1)))
	})

	It("should trace three tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1})
		t.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		t.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		Expect(len(t.tracingTasks)).To(Equal(3))
		Expect(t.tracingTasks[1].ParentID).To(Equal(uint64(0)))
		Expect(t.tracingTasks[2].ParentID).To(Equal(uint64(1)))
		Expect(t.tracingTasks[3].ParentID).To(Equal(uint64(2)))
	})

	It("should end a single task", func() {
		t.TaskStarted(tasks.Task{ID: 1})

		t.TaskEnded(tasks.Task{ID: 1})

		Expect(len(t.tracingTasks)).To(Equal(0))
	})

	It("should end two tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1})
		t.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		t.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		t.TaskEnded(tasks.Task{ID: 3})
		t.TaskEnded(tasks.Task{ID: 2})

		Expect(len(t.tracingTasks)).To(Equal(1))
		Expect(t.tracingTasks[1].ParentID).To(Equal(uint64(0)))
	})

	It("should print single tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1})

		mockTaskPrinter.EXPECT().Print(tasks.Task{ID: 1})

		t.DumpBackTrace(tasks.Task{ID: 1})
	})

	It("should print three tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1})
		t.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		t.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		e3 := mockTaskPrinter.EXPECT().
			Print(tasks.Task{ID: 3, ParentID: 2})
		e2 := mockTaskPrinter.EXPECT().
			Print(tasks.Task{ID: 2, ParentID: 1}).
			After(e3)
		mockTaskPrinter.EXPECT().Print(tasks.Task{ID: 1}).
			After(e2)

		t.DumpBackTrace(tasks.Task{ID: 3, ParentID: 2})
	})

	It("should print three tasks", func() {
		t.TaskStarted(tasks.Task{ID: 1})
		t.TaskStarted(tasks.Task{ID: 2, ParentID: 1})
		t.TaskStarted(tasks.Task{ID: 3, ParentID: 2})

		t.TaskEnded(tasks.Task{ID: 2})

		mockTaskPrinter.EXPECT().
			Print(tasks.Task{ID: 3, ParentID: 2})

		t.DumpBackTrace(tasks.Task{ID: 3, ParentID: 2})
	})
})
