package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BufferImpl", func() {

	var (
		buf Buffer
	)

	BeforeEach(func() {
		buf = NewBuffer("Buf", 2)
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
