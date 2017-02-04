package conn_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/conn/mock_conn"
)

var _ = Describe("DirectConnection", func() {

	var (
		mockCtrl   *gomock.Controller
		comp1      *mock_conn.MockComponent
		comp2      *mock_conn.MockComponent
		comp3      *mock_conn.MockComponent
		connection *conn.DirectConnection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		comp1 = mock_conn.NewMockComponent(mockCtrl)
		comp2 = mock_conn.NewMockComponent(mockCtrl)
		comp3 = mock_conn.NewMockComponent(mockCtrl)

		connection = conn.NewDirectConnection()
		connection.Register(comp1)
		connection.Register(comp2)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("when destination is specified", func() {
		It("should check the receiver for can or cannot send", func() {
			req := conn.Request{}
			req.From = comp1
			req.To = comp2

			comp2.EXPECT().CanRecv(&req).Return(nil)
			Expect(connection.CanSend(&req)).To(BeNil())

			err := conn.NewConnError("error", false, 10)
			comp2.EXPECT().CanRecv(&req).Return(err)
			Expect(connection.CanSend(&req)).To(BeIdenticalTo(err))
		})

		It("should give an error if the source is not connected", func() {
			req := conn.Request{}
			req.From = comp3

			comp3.EXPECT().Name().Return("comp3")

			err := connection.CanSend(&req)
			Expect(err).NotTo(BeNil())
			Expect(err.Recoverable).To(BeFalse())

		})

		It("should give an error if the destination is not connected", func() {
			req := conn.Request{}
			req.From = comp1
			req.To = comp3

			comp3.EXPECT().Name().Return("comp3")

			err := connection.CanSend(&req)
			Expect(err).NotTo(BeNil())
			Expect(err.Recoverable).To(BeFalse())
		})

		It("should send", func() {
			req := conn.Request{}
			req.From = comp1
			req.To = comp2

			comp2.EXPECT().Recv(&req).Return(nil)

			err := connection.Send(&req)
			Expect(err).To(BeNil())
		})

		It("should return the error that the receiver return", func() {
			req := conn.Request{}
			req.From = comp1
			req.To = comp2

			err := conn.NewConnError("error", false, 10)
			comp2.EXPECT().Recv(&req).Return(err)

			Expect(connection.Send(&req)).To(BeIdenticalTo(err))

		})
	})

	Context("when the destination is specified", func() {

		It("should check the receiver, if the connection is one to one", func() {
			req := conn.Request{}
			req.From = comp1

			comp2.EXPECT().CanRecv(&req).Return(nil)
			Expect(connection.CanSend(&req)).To(BeNil())

			err := conn.NewConnError("error", false, 10)
			comp2.EXPECT().CanRecv(&req).Return(err)
			Expect(connection.CanSend(&req)).To(BeIdenticalTo(err))
		})

		It("should give and error if the connection is not one-to-one", func() {
			connection.Register(comp3)

			req := conn.Request{}
			req.From = comp1

			err := connection.CanSend(&req)
			Expect(err).NotTo(BeNil())
			Expect(err.Recoverable).To(BeFalse())
		})

		It("should give error when sending", func() {
			req := conn.Request{}
			req.From = comp1

			Expect(connection.Send(&req)).NotTo(BeNil())
		})

	})

})
