package requestsys_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/requestsys"
	"gitlab.com/yaotsu/core/requestsys/mock_requestsys"
)

var _ = Describe("DirectConnection", func() {

	var (
		mockCtrl *gomock.Controller
		comp1    *mock_requestsys.MockComponent
		comp2    *mock_requestsys.MockComponent
		comp3    *mock_requestsys.MockComponent
		conn     *requestsys.DirectConnection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		comp1 = mock_requestsys.NewMockComponent(mockCtrl)
		comp2 = mock_requestsys.NewMockComponent(mockCtrl)
		comp3 = mock_requestsys.NewMockComponent(mockCtrl)

		conn = requestsys.NewDirectConnection()
		conn.Register(comp1)
		conn.Register(comp2)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("when destination is specified", func() {

		It("should check the receiver for can or cannot send", func() {
			req := requestsys.Request{}
			req.From = comp1
			req.To = comp2

			comp2.EXPECT().CanRecv(&req).Return(nil)
			Expect(conn.CanSend(&req)).To(BeNil())

			err := requestsys.NewConnError("error", false, 10)
			comp2.EXPECT().CanRecv(&req).Return(err)
			Expect(conn.CanSend(&req)).To(BeIdenticalTo(err))
		})
	})
})
