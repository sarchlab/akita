package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// countingHook is a CommandHook that records the kind of every issued command.
// It only observes — it must not change scheduling results.
type countingHook struct {
	count    int
	byKind   map[commandKind]int
	lastTick uint64
}

func newCountingHook() *countingHook {
	return &countingHook{byKind: map[commandKind]int{}}
}

func (*countingHook) Name() string { return "counting" }

func (h *countingHook) OnIssue(_ *Spec, _ *State, cmd *commandState, now uint64) {
	h.count++
	h.byKind[commandKind(cmd.Kind)]++
	h.lastTick = now
}

var _ = Describe("P1: plugin selection", func() {
	It("selects the default plugins from a default spec", func() {
		spec := DefaultSpec()
		ctrl := newDefaultController(&spec)

		Expect(ctrl.scheduler.Name()).To(Equal("FRFCFS"))
		Expect(ctrl.refresh.Name()).To(Equal("fakestall"))
		Expect(ctrl.addrMapper.Name()).To(Equal("default"))
		Expect(ctrl.hooks).To(BeEmpty())
	})

	It("derives the row policy from PagePolicy", func() {
		open := DefaultSpec()
		open.PagePolicy = PagePolicyOpen
		Expect(newDefaultController(&open).rowPolicy.Name()).To(Equal("open"))

		closed := DefaultSpec()
		closed.PagePolicy = PagePolicyClose
		Expect(newDefaultController(&closed).rowPolicy.Name()).To(Equal("close"))
	})

	It("lets builder overrides win over the spec selection", func() {
		spec := DefaultSpec()
		spec.PagePolicy = PagePolicyClose

		ctrl := MakeBuilder().
			WithSpec(spec).
			WithRowPolicy(openPageRowPolicy{}).
			WithPlugin(nullCommandHook{}).
			buildController()

		Expect(ctrl.rowPolicy.Name()).To(Equal("open"))
		Expect(ctrl.hooks).To(HaveLen(1))
	})

	It("panics on an unknown registry key", func() {
		spec := DefaultSpec()
		spec.Scheduler = "does-not-exist"
		Expect(func() { newDefaultController(&spec) }).To(Panic())
	})
})

var _ = Describe("P1: command hooks", func() {
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

		// The hook saw real command activity (at least ACT + column commands).
		Expect(hook.count).To(BeNumerically(">", 0))
		Expect(hook.byKind[cmdKindActivate]).To(BeNumerically(">", 0))
		Expect(hook.lastTick).To(BeNumerically(">", 0))

		// Results are identical to the no-hook run.
		Expect(hookedWrites).To(HaveLen(len(plainWrites)))
		Expect(hookedReads).To(HaveLen(len(plainReads)))
		Expect(hookedReads).To(HaveLen(1))
		Expect(hookedReads[0].Data[:4]).To(Equal([]byte{1, 2, 3, 4}))
	})
})
