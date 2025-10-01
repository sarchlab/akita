package queueingv5

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buffer", func() {
	var buffer *Buffer

	BeforeEach(func() {
		buffer = NewBuffer("TestBuffer", 3)
	})

	Context("when newly created", func() {
		It("should have correct name", func() {
			Expect(buffer.Name()).To(Equal("TestBuffer"))
		})

		It("should have correct capacity", func() {
			Expect(buffer.Capacity()).To(Equal(3))
		})

		It("should be empty", func() {
			Expect(buffer.Size()).To(Equal(0))
		})

		It("should be able to push", func() {
			Expect(buffer.CanPush()).To(BeTrue())
		})

		It("should return nil when peeking", func() {
			Expect(buffer.Peek()).To(BeNil())
		})

		It("should return nil when popping", func() {
			Expect(buffer.Pop()).To(BeNil())
		})
	})

	Context("when elements are added", func() {
		BeforeEach(func() {
			buffer.Push("item1")
			buffer.Push("item2")
		})

		It("should have correct size", func() {
			Expect(buffer.Size()).To(Equal(2))
		})

		It("should peek the first element", func() {
			Expect(buffer.Peek()).To(Equal("item1"))
		})

		It("should pop elements in FIFO order", func() {
			item1 := buffer.Pop()
			Expect(item1).To(Equal("item1"))
			Expect(buffer.Size()).To(Equal(1))

			item2 := buffer.Pop()
			Expect(item2).To(Equal("item2"))
			Expect(buffer.Size()).To(Equal(0))
		})
	})

	Context("when buffer is full", func() {
		BeforeEach(func() {
			buffer.Push("item1")
			buffer.Push("item2")
			buffer.Push("item3")
		})

		It("should not be able to push", func() {
			Expect(buffer.CanPush()).To(BeFalse())
		})

		It("should panic when trying to push beyond capacity", func() {
			Expect(func() {
				buffer.Push("item4")
			}).To(Panic())
		})
	})

	Context("when cleared", func() {
		BeforeEach(func() {
			buffer.Push("item1")
			buffer.Push("item2")
			buffer.Clear()
		})

		It("should be empty", func() {
			Expect(buffer.Size()).To(Equal(0))
		})

		It("should be able to push", func() {
			Expect(buffer.CanPush()).To(BeTrue())
		})
	})
})