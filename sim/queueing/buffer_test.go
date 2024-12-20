package queueing

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BufferImpl", func() {
	var (
		mockCtrl *gomock.Controller
		sim      *MockSimulation
		buf      Buffer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().RegisterStateHolder(gomock.Any())

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
