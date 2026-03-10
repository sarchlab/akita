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
		component = NewComponentBase("TestComp")
	})

	It("should set and get name", func() {
		Expect(component.Name()).To(Equal("TestComp"))
	})
})
