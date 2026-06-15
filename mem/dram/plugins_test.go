package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
)

// countingHook is an Akita hook that records issued commands via the
// HookPosCmdIssued hook point. It only observes — it must not change results.
type countingHook struct {
	count    int
	byKind   map[string]int
	lastTick uint64
}

func newCountingHook() *countingHook {
	return &countingHook{byKind: map[string]int{}}
}

func (h *countingHook) Func(ctx hooking.HookCtx) {
	if ctx.Pos != HookPosCmdIssued {
		return
	}
	ev := ctx.Item.(CommandEvent)
	h.count++
	h.byKind[ev.Kind]++
	h.lastTick = ev.Tick
}

var _ = Describe("P1: strategy selection", func() {
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

var _ = Describe("P1: command-issued hook", func() {
	It("observes issued commands without changing results", func() {
		spec := DefaultSpec()
		hook := newCountingHook()

		// Same workload, with and without the hook.
		withHook := newP0Harness(spec, hook)
		withHook.src.Send(withHook.write(0x40, []byte{1, 2, 3, 4}))
		withHook.engine.Run()
		withHook.src.Send(withHook.read(0x40))
		withHook.engine.Run()
		hookedReads, hookedWrites := withHook.collect()

		plain := newP0Harness(spec)
		plain.src.Send(plain.write(0x40, []byte{1, 2, 3, 4}))
		plain.engine.Run()
		plain.src.Send(plain.read(0x40))
		plain.engine.Run()
		plainReads, plainWrites := plain.collect()

		// The hook saw real command activity (at least an ACT + column commands).
		Expect(hook.count).To(BeNumerically(">", 0))
		Expect(hook.byKind["ACT"]).To(BeNumerically(">", 0))
		Expect(hook.lastTick).To(BeNumerically(">", 0))

		// Results are identical to the no-hook run.
		Expect(hookedWrites).To(HaveLen(len(plainWrites)))
		Expect(hookedReads).To(HaveLen(len(plainReads)))
		Expect(hookedReads).To(HaveLen(1))
		Expect(hookedReads[0].Data[:4]).To(Equal([]byte{1, 2, 3, 4}))
	})
})
