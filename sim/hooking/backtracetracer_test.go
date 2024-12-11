package hooking

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type stubTaskPrinter struct {
	printed []task
}

func (p *stubTaskPrinter) Print(task task) {
	p.printed = append(p.printed, task)
}

var _ = Describe("BackTraceTracer", func() {
	var (
		mockCtrl        *gomock.Controller
		mockTaskPrinter *stubTaskPrinter
		t               *BackTraceTracer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockTaskPrinter = &stubTaskPrinter{}

		t = NewBackTraceTracer(mockTaskPrinter)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should trace a single task", func() {
		t.StartTask(TaskStart{ID: "1"})

		Expect(len(t.tracingTasks)).To(Equal(1))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
	})

	It("should trace two tasks", func() {
		t.StartTask(TaskStart{ID: "1"})
		t.StartTask(TaskStart{ID: "2", ParentID: "1"})

		Expect(len(t.tracingTasks)).To(Equal(2))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
		Expect(t.tracingTasks["2"].ParentID).To(Equal("1"))
	})

	It("should trace three tasks", func() {
		t.StartTask(TaskStart{ID: "1"})
		t.StartTask(TaskStart{ID: "2", ParentID: "1"})
		t.StartTask(TaskStart{ID: "3", ParentID: "2"})

		Expect(len(t.tracingTasks)).To(Equal(3))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
		Expect(t.tracingTasks["2"].ParentID).To(Equal("1"))
		Expect(t.tracingTasks["3"].ParentID).To(Equal("2"))
	})

	It("should end a single task", func() {
		t.StartTask(TaskStart{ID: "1"})

		t.EndTask(TaskEnd{ID: "1"})

		Expect(len(t.tracingTasks)).To(Equal(0))
	})

	It("should end two tasks", func() {
		t.StartTask(TaskStart{ID: "1"})
		t.StartTask(TaskStart{ID: "2", ParentID: "1"})
		t.StartTask(TaskStart{ID: "3", ParentID: "2"})

		t.EndTask(TaskEnd{ID: "3"})
		t.EndTask(TaskEnd{ID: "2"})

		Expect(len(t.tracingTasks)).To(Equal(1))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
	})

	It("should print single tasks", func() {
		t.StartTask(TaskStart{ID: "1"})

		t.DumpBackTrace("1")

		Expect(mockTaskPrinter.printed).To(Equal([]task{{ID: "1"}}))
	})

	It("should print three tasks", func() {
		t.StartTask(TaskStart{ID: "1"})
		t.StartTask(TaskStart{ID: "2", ParentID: "1"})
		t.StartTask(TaskStart{ID: "3", ParentID: "2"})

		t.DumpBackTrace("3")

		Expect(mockTaskPrinter.printed).To(Equal([]task{
			{ID: "3", ParentID: "2"},
			{ID: "2", ParentID: "1"},
			{ID: "1"},
		}))
	})

	It("should print three tasks", func() {
		t.StartTask(TaskStart{ID: "1"})
		t.StartTask(TaskStart{ID: "2", ParentID: "1"})
		t.StartTask(TaskStart{ID: "3", ParentID: "2"})

		t.EndTask(TaskEnd{ID: "2"})

		t.DumpBackTrace("3")

		Expect(mockTaskPrinter.printed).To(Equal([]task{
			{ID: "3", ParentID: "2"},
			{ID: "2"},
			{ID: "1"},
		}))
	})
})
