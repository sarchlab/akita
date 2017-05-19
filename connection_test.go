package core_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("PlugIn", func() {
	It("should link a connection and a component", func() {
		comp := core.NewMockComponent("comp")
		comp.AddPort("Port")
		connection := core.NewMockConnection()

		core.PlugIn(comp, "Port", connection)

		Expect(comp.GetConnection("Port")).To(BeIdenticalTo(connection))
	})
})
