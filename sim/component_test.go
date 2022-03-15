package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BasicComponent", func() {

	var (
		component *ComponentBase
	)

	BeforeEach(func() {
		component = NewComponentBase("test_comp")
	})

	It("should set and get name", func() {
		Expect(component.Name()).To(Equal("test_comp"))
	})
})
