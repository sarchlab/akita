package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TokeniedName", func() {
	It("should parse name", func() {
		name := ParseName("GPU[0].Core[0]")
		Expect(name.Tokens[0].ElemName).To(Equal("GPU"))
		Expect(name.Tokens[0].Index).To(Equal([]int{0}))
		Expect(name.Tokens[1].ElemName).To(Equal("Core"))
		Expect(name.Tokens[1].Index).To(Equal([]int{0}))
	})

	It("should parse multi-dimensional index", func() {
		name := ParseName("GPU[0][1].Core[0][1]")
		Expect(name.Tokens[0].ElemName).To(Equal("GPU"))
		Expect(name.Tokens[0].Index).To(Equal([]int{0, 1}))
		Expect(name.Tokens[1].ElemName).To(Equal("Core"))
		Expect(name.Tokens[1].Index).To(Equal([]int{0, 1}))
	})

	It("should panic if the name is empty", func() {
		Expect(func() { NameMustBeValid("") }).To(Panic())
	})

	It("should panic if name include underscore", func() {
		Expect(func() { NameMustBeValid("GPU_0") }).To(Panic())
	})

	It("should panic if name include dash", func() {
		Expect(func() { NameMustBeValid("GPU-0") }).To(Panic())
	})

	It("should panic if name is not capitalized CamelCase", func() {
		Expect(func() { NameMustBeValid("gpu0") }).To(Panic())
	})

	It("should have paired square brackets", func() {
		Expect(func() { NameMustBeValid("GPU[0") }).To(Panic())
	})

	It("should have paired square brackets", func() {
		Expect(func() { NameMustBeValid("GPU0]") }).To(Panic())
	})

	It("should be panic if element name is empty", func() {
		Expect(func() { NameMustBeValid("GPU..0") }).To(Panic())
	})

	It("should build name", func() {
		Expect(BuildName("", "GPU")).To(Equal("GPU"))
		Expect(BuildName("GPU", "Core")).To(Equal("GPU.Core"))
	})

	It("should build name with index", func() {
		Expect(BuildNameWithIndex("", "GPU", 0)).To(Equal("GPU[0]"))
		Expect(BuildNameWithIndex("GPU", "Core", 0)).To(Equal("GPU.Core[0]"))
	})

	It("should build name with multi-dimensional index", func() {
		Expect(BuildNameWithMultiDimensionalIndex("", "GPU", []int{0})).
			To(Equal("GPU[0]"))
		Expect(BuildNameWithMultiDimensionalIndex("GPU", "Core", []int{0, 1})).
			To(Equal("GPU.Core[0][1]"))
	})
})
