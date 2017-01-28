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

		It("should give an error if the source is not connected", func() {
			req := requestsys.Request{}
			req.From = comp3

			comp3.EXPECT().Name().Return("comp3")

			err := conn.CanSend(&req)
			Expect(err).NotTo(BeNil())
			Expect(err.Recoverable).To(BeFalse())

		})

		It("should give an error if the destination is not connected", func() {
			req := requestsys.Request{}
			req.From = comp1
			req.To = comp3

			comp3.EXPECT().Name().Return("comp3")

			err := conn.CanSend(&req)
			Expect(err).NotTo(BeNil())
			Expect(err.Recoverable).To(BeFalse())
		})
	})

	Context("when the connection is one-to-one", func() {

		It("should check the receiver for can or cannot send", func() {
			req := requestsys.Request{}
			req.From = comp1

			comp2.EXPECT().CanRecv(&req).Return(nil)
			Expect(conn.CanSend(&req)).To(BeNil())

			err := requestsys.NewConnError("error", false, 10)
			comp2.EXPECT().CanRecv(&req).Return(err)
			Expect(conn.CanSend(&req)).To(BeIdenticalTo(err))
		})

		It("should give and error if the connection is not one-to-one", func() {
			conn.Register(comp3)

			req := requestsys.Request{}
			req.From = comp1

			err := conn.CanSend(&req)

			Expect(err).NotTo(BeNil())
			Expect(err.Recoverable).To(BeFalse())
		})

	})

	Context("when the destination is not known", func() {
		It("should not allow sending", func() {
			conn.Register(comp3)
			req := requestsys.Request{}
			req.From = comp1

			err := conn.CanSend(&req)
			Expect(err).NotTo(BeNil())
			Expect(err.Recoverable).To(BeFalse())
		})

	})
})
