package akita

import (
	. "github.com/onsi/ginkgo"
)

var _ = Describe("DirectConnection", func() {

	var (
		port1      Port
		port2      Port
		connection *DirectConnection
		engine     *MockEngine
	)

	BeforeEach(func() {
		port1 = NewLimitNumReqPort(nil, 1)
		port2 = NewLimitNumReqPort(nil, 1)
		engine = NewMockEngine()

		connection = NewDirectConnection(engine)
		connection.PlugIn(port1)
		connection.PlugIn(port2)
	})

})
