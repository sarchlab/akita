package tracing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BackTraceTracer", func() {
	var (
		t *BackTraceTracer
	)

	BeforeEach(func() {
		t = NewBackTraceTracer()
	})

	It("should trace a single task", func() {
		t.StartTask(Task{ID: "1"})

		Expect(len(t.tracingTasks)).To(Equal(1))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
	})

	It("should trace two tasks", func() {
		t.StartTask(Task{ID: "1"})
		t.StartTask(Task{ID: "2", ParentID: "1"})

		Expect(len(t.tracingTasks)).To(Equal(2))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
		Expect(t.tracingTasks["2"].ParentID).To(Equal("1"))
	})

	It("should trace three tasks", func() {
		t.StartTask(Task{ID: "1"})
		t.StartTask(Task{ID: "2", ParentID: "1"})
		t.StartTask(Task{ID: "3", ParentID: "2"})

		Expect(len(t.tracingTasks)).To(Equal(3))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
		Expect(t.tracingTasks["2"].ParentID).To(Equal("1"))
		Expect(t.tracingTasks["3"].ParentID).To(Equal("2"))
	})

	It("should end a single task", func() {
		t.StartTask(Task{ID: "1"})

		t.EndTask(Task{ID: "1"})

		Expect(len(t.tracingTasks)).To(Equal(0))
	})

	It("should end two tasks", func() {
		t.StartTask(Task{ID: "1"})
		t.StartTask(Task{ID: "2", ParentID: "1"})
		t.StartTask(Task{ID: "3", ParentID: "2"})

		t.EndTask(Task{ID: "3", ParentID: "2"})
		t.EndTask(Task{ID: "2", ParentID: "1"})

		Expect(len(t.tracingTasks)).To(Equal(1))
		Expect(t.tracingTasks["1"].ParentID).To(Equal(""))
	})

	It("should print single tasks", func() {
		t.StartTask(Task{ID: "1"})

		t.DumpBackTrace(Task{ID: "1"})
	})

	It("should print three tasks", func() {
		t.StartTask(Task{ID: "1"})
		t.StartTask(Task{ID: "2", ParentID: "1"})
		t.StartTask(Task{ID: "3", ParentID: "2"})

		t.DumpBackTrace(Task{ID: "3", ParentID: "2"})
	})
})
