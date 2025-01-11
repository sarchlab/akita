package hooking

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

type stubTaskPrinter struct {
	printed []task
}

func (p *stubTaskPrinter) Print(task task) {
	p.printed = append(p.printed, task)
}

type stubHookable struct {
}

func (h *stubHookable) AcceptHook(hook Hook) {
}

func (h *stubHookable) NumHooks() int {
	return 0
}

func (h *stubHookable) Hooks() []Hook {
	return nil
}

func (h *stubHookable) Name() string {
	return "stub"
}

var _ = Describe("BackTraceTracer", func() {
	var (
		mockCtrl        *gomock.Controller
		mockTaskPrinter *stubTaskPrinter
		mockHookable    *stubHookable
		t               *BackTraceTracer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockTaskPrinter = &stubTaskPrinter{}
		mockHookable = &stubHookable{}
		t = NewBackTraceTracer(mockTaskPrinter)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should print three tasks", func() {
		ctx1 := HookCtx{
			Domain: mockHookable,
			Pos:    HookPosTaskStart,
			Item:   TaskStart{ID: "1"},
		}
		t.StartTask(ctx1)

		ctx2 := HookCtx{
			Domain: mockHookable,
			Pos:    HookPosTaskStart,
			Item:   TaskStart{ID: "2", ParentID: "1"},
		}
		t.StartTask(ctx2)

		ctx3 := HookCtx{
			Domain: mockHookable,
			Pos:    HookPosTaskStart,
			Item:   TaskStart{ID: "3", ParentID: "2"},
		}
		t.StartTask(ctx3)

		t.DumpBackTrace("3")

		Expect(len(t.tracingTasks)).To(Equal(3))
		Expect(mockTaskPrinter.printed).To(HaveLen(3))
		Expect(mockTaskPrinter.printed[0]).
			To(Equal(task{ID: "3", ParentID: "2", Where: "stub"}))
		Expect(mockTaskPrinter.printed[1]).
			To(Equal(task{ID: "2", ParentID: "1", Where: "stub"}))
		Expect(mockTaskPrinter.printed[2]).
			To(Equal(task{ID: "1", ParentID: "", Where: "stub"}))
	})

	It("should print as many tasks as possible", func() {
		ctx1 := HookCtx{
			Domain: mockHookable,
			Pos:    HookPosTaskStart,
			Item:   TaskStart{ID: "1"},
		}
		t.StartTask(ctx1)

		ctx2 := HookCtx{
			Domain: mockHookable,
			Pos:    HookPosTaskStart,
			Item:   TaskStart{ID: "2", ParentID: "1"},
		}
		t.StartTask(ctx2)

		ctx3 := HookCtx{
			Domain: mockHookable,
			Pos:    HookPosTaskStart,
			Item:   TaskStart{ID: "3", ParentID: "2"},
		}
		t.StartTask(ctx3)

		ctx4 := HookCtx{
			Domain: mockHookable,
			Pos:    HookPosTaskEnd,
			Item:   TaskEnd{ID: "2"},
		}
		t.EndTask(ctx4)

		t.DumpBackTrace("3")

		Expect(mockTaskPrinter.printed).To(HaveLen(1))
		Expect(mockTaskPrinter.printed[0]).
			To(Equal(task{ID: "3", ParentID: "2", Where: "stub"}))
	})
})
