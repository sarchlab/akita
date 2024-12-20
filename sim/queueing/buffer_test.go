package queueing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim/simulation"
)

var _ = Describe("BufferImpl", func() {
	var (
		sim *simulation.Simulation
		buf Buffer
	)

	BeforeEach(func() {
		sim = simulation.NewSimulation()
		buf = BufferBuilder{}.
			WithSimulation(sim).
			WithCapacity(2).
			Build("Buf")
	})

	It("should allow push and pop", func() {
		Expect(buf.Capacity()).To(Equal(2))
		Expect(buf.CanPush()).To(BeTrue())

		buf.Push(1)
		Expect(buf.CanPush()).To(BeTrue())
		Expect(buf.Size()).To(Equal(1))

		buf.Push(2)
		Expect(buf.CanPush()).To(BeFalse())
		Expect(buf.Size()).To(Equal(2))
		Expect(func() {
			buf.Push(3)
		}).To(Panic())

		Expect(buf.Peek()).To(Equal(1))
		Expect(buf.Pop()).To(Equal(1))
		Expect(buf.Size()).To(Equal(1))
		Expect(buf.Peek()).To(Equal(2))
		Expect(buf.Pop()).To(Equal(2))
		Expect(buf.Size()).To(Equal(0))
		Expect(buf.Peek()).To(BeNil())
		Expect(buf.Pop()).To(BeNil())
	})

	It("should clear", func() {
		buf.Push(2)
		Expect(buf.Size()).To(Equal(1))

		buf.Clear()

		Expect(buf.Size()).To(Equal(0))
		Expect(buf.Peek()).To(BeNil())
	})
})
