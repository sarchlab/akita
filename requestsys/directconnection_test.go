package requestsys_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/requestsys"
)

var _ = Describe("DirectConnection", func() {
	Context("when destination is specified", func() {
		var comp1 *mockComponent
		var comp2 *mockComponent
		var conn *requestsys.DirectConnection

		BeforeEach(func() {
			comp1 = newMockComponent("mock_comp_1")
			comp2 = newMockComponent("mock_comp_2")
			comp1.AddPort("port")
			comp2.AddPort("port")

			conn = requestsys.NewDirectConnection()
			comp1.Connect("port", conn)
			comp2.Connect("port", conn)
			conn.Register(comp1)
			conn.Register(comp2)
		})

		It("should check the receiver for can or cannot send", func() {
			req := requestsys.Request{}
			req.From = comp1
			req.To = comp2

			comp2.canRecv = true
			Expect(conn.CanSend(&req)).To(BeTrue())

			comp2.canRecv = false
			Expect(conn.CanSend(&req)).To(BeFalse())
		})
	})
})
