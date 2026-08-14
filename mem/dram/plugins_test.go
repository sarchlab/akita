package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/tracing"
)

// cmdTracer is a Tracer that counts the command-issue milestones recorded on
// sub-transaction tasks. It only observes — it must not change results.
type cmdTracer struct {
	tracing.NopTracer
	count    int
	byKind   map[string]int
	subTasks int
}

func newCmdTracer() *cmdTracer {
	return &cmdTracer{byKind: map[string]int{}}
}

func (t *cmdTracer) StartTask(ts tracing.TaskStart) {
	if ts.Kind == "sub-trans" {
		t.subTasks++
	}
}

func (t *cmdTracer) AddMilestone(m tracing.Milestone) {
	t.count++
	t.byKind[m.What]++
}

var _ = Describe("strategy selection", func() {
	It("selects the default strategies from a default spec", func() {
		spec := DefaultSpec()
		ctrl := newDefaultController(&spec)

		Expect(ctrl.scheduler.Name()).To(Equal("FRFCFS"))
		Expect(ctrl.addrMapper.Name()).To(Equal("default"))
	})

	It("derives the row policy from PagePolicy", func() {
		open := DefaultSpec()
		open.PagePolicy = PagePolicyOpen
		Expect(newDefaultController(&open).rowPolicy.Name()).To(Equal("open"))

		closed := DefaultSpec()
		closed.PagePolicy = PagePolicyClose
		Expect(newDefaultController(&closed).rowPolicy.Name()).To(Equal("close"))
	})

	It("selects a scheduler by its Spec registry key", func() {
		spec := DefaultSpec()
		spec.Scheduler = "FRFCFS"
		Expect(newDefaultController(&spec).scheduler.Name()).To(Equal("FRFCFS"))
	})

	It("panics on an unknown registry key", func() {
		spec := DefaultSpec()
		spec.Scheduler = "does-not-exist"
		Expect(func() { newDefaultController(&spec) }).To(Panic())
	})
})

var _ = Describe("command tracing", func() {
	It("records command milestones without changing results", func() {
		spec := DefaultSpec()
		tracer := newCmdTracer()

		// Same workload, with and without the tracer.
		traced := newDramHarness(spec, tracer)
		traced.src.Send(traced.write(0x40, []byte{1, 2, 3, 4}))
		traced.engine.Run()
		traced.src.Send(traced.read(0x40))
		traced.engine.Run()
		tracedReads, tracedWrites := traced.collect()

		plain := newDramHarness(spec)
		plain.src.Send(plain.write(0x40, []byte{1, 2, 3, 4}))
		plain.engine.Run()
		plain.src.Send(plain.read(0x40))
		plain.engine.Run()
		plainReads, plainWrites := plain.collect()

		// The tracer saw sub-transaction tasks and command milestones (at least
		// an ACT plus column commands) on them.
		Expect(tracer.subTasks).To(BeNumerically(">", 0))
		Expect(tracer.count).To(BeNumerically(">", 0))
		Expect(tracer.byKind["ACT"]).To(BeNumerically(">", 0))

		// Results are identical to the untraced run.
		Expect(tracedWrites).To(HaveLen(len(plainWrites)))
		Expect(tracedReads).To(HaveLen(len(plainReads)))
		Expect(tracedReads).To(HaveLen(1))
		Expect(tracedReads[0].Data[:4]).To(Equal([]byte{1, 2, 3, 4}))
	})
})
