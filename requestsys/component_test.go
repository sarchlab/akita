package requestsys_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/requestsys"
)

var _ = Describe("BasicComponent", func() {
	component := newMockComponent()

	It("should set and get name", func() {
		Expect(component.Name()).To(Equal("mock"))
	})

	It("should find socket by name", func() {
		socket := requestsys.NewSocket("test_sock")

		retSocket := component.GetSocketByName("test_sock")
		Expect(retSocket).To(BeNil())

		requestsys.BindSocket(component, socket)
		retSocket = component.GetSocketByName("test_sock")
		Expect(retSocket).To(BeIdenticalTo(socket))
	})
})
