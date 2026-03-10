package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IDGenerator", func() {
	BeforeEach(func() {
		ResetIDGenerator()
	})

	AfterEach(func() {
		ResetIDGenerator()
	})

	It("should get and set the next ID", func() {
		UseSequentialIDGenerator()

		SetIDGeneratorNextID(100)
		Expect(GetIDGeneratorNextID()).To(Equal(uint64(100)))
	})

	It("should reset the ID generator", func() {
		UseSequentialIDGenerator()
		SetIDGeneratorNextID(42)

		ResetIDGenerator()

		// After reset, we can create a new generator
		gen := GetIDGenerator()
		Expect(gen).NotTo(BeNil())
		Expect(GetIDGeneratorNextID()).To(Equal(uint64(0)))
	})

	It("should get next ID that reflects Generate calls", func() {
		UseSequentialIDGenerator()

		gen := GetIDGenerator()
		gen.Generate()
		gen.Generate()

		Expect(GetIDGeneratorNextID()).To(Equal(uint64(2)))
	})
})
