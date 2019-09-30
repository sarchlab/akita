package akita

import (
	. "github.com/onsi/ginkgo"
)

var _ = Describe("DirectConnection", func() {

	var (
		port1      Port
		port2      Port
		connection *DirectConnection
	)

	BeforeEach(func() {
		port1 = NewLimitNumMsgPort(nil, 1, "port1")
		port2 = NewLimitNumMsgPort(nil, 1, "port2")

		connection.PlugIn(port1)
		connection.PlugIn(port2)
	})

})
