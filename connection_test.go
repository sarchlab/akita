package core

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PlugIn", func() {
	It("should link a connection and a component", func() {
		comp := NewMockComponent("comp")
		comp.AddPort("Port")
		connection := NewMockConnection()

		PlugIn(comp, "Port", connection)

		Expect(comp.GetConnection("Port")).To(BeIdenticalTo(connection))
	})
})
