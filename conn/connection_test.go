package conn_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
)

var _ = Describe("PlugIn", func() {
	It("should link a connection and a component", func() {
		comp := conn.NewMockComponent()
		comp.AddPort("Port")
		connection := NewMockConnection()

		err := conn.PlugIn(comp, "Port", connection)

		Expect(err).To(BeNil())
		Expect(connection.Connected[comp]).To(BeTrue())
		Expect(comp.GetConnection("Port")).To(BeIdenticalTo(connection))
	})
})
