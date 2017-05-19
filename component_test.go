package core_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/core"
)

var _ = Describe("BasicComponent", func() {

	var (
		component  *core.ComponentBase
		connection *core.MockConnection
	)

	BeforeEach(func() {
		component = core.NewComponentBase("test_comp")
		connection = core.NewMockConnection()
	})

	It("should set and get name", func() {
		Expect(component.Name()).To(Equal("test_comp"))
	})

	It("should not accept empty port name", func() {
		Expect(func() { component.AddPort("") }).To(Panic())
	})

	It("should not accept duplicate port name", func() {
		component.AddPort("port1")
		Expect(func() { component.AddPort("port1") }).To(Panic())
	})

	It("should panic if connecting to non-exist port", func() {
		component.AddPort("port")
		Expect(func() { component.Connect("port2", nil) }).To(Panic())
	})

	It("should connect port with connection", func() {
		component.AddPort("port")

		component.Connect("port", connection)

		Expect(component.GetConnection("port")).To(BeIdenticalTo(connection)) })

	It("should panic if disconnecting a non-exist port", func() {
		component.AddPort("port")
		Expect(func() { component.Disconnect("port2") }).To(Panic())
	})

	It("should panic if disconnecting a port that is not connected", func() {
		component.AddPort("port")
		Expect(func() { component.Disconnect("port") }).To(Panic())
	})

	It("should disconnect port", func() {
		component.AddPort("port")

		component.Connect("port", connection)
		Expect(component.GetConnection("port")).To(BeIdenticalTo(connection))

		component.Disconnect("port")
		Expect(component.GetConnection("port")).To(BeNil())
	})

})
