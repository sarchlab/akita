package requestsys_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/requestsys"
	"gitlab.com/yaotsu/core/requestsys/mock_requestsys"
)

var _ = Describe("BasicComponent", func() {

	var (
		mockCtrl  *gomock.Controller
		component *requestsys.BasicComponent
		conn      *mock_requestsys.MockConnection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		component = requestsys.NewBasicComponent("test_comp")
		conn = mock_requestsys.NewMockConnection(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should set and get name", func() {
		Expect(component.Name()).To(Equal("test_comp"))
	})

	It("should not accept empty port name", func() {
		Expect(component.AddPort("")).NotTo(BeNil())
	})

	It("should not accept duplicate port name", func() {
		Expect(component.AddPort("port1")).To(BeNil())
		Expect(component.AddPort("port1")).NotTo(BeNil())
	})

	It("should give error if connecting to non-exist port", func() {
		component.AddPort("port")
		Expect(component.Connect("port2", nil)).ToNot(BeNil())
	})

	It("should connect port with connection", func() {
		component.AddPort("port")

		component.Connect("port", conn)

		Expect(component.GetConnection("port")).To(BeIdenticalTo(conn))

	})

	It("should give error if disconnecting a non-exist port", func() {
		component.AddPort("port")
		Expect(component.Disconnect("port2")).NotTo(BeNil())
	})

	It("should give error if disconnecting a port that is not connected", func() {
		component.AddPort("port")
		Expect(component.Disconnect("port")).NotTo(BeNil())
	})

	It("should disconnect port", func() {
		component.AddPort("port")

		component.Connect("port", conn)
		Expect(component.GetConnection("port")).To(BeIdenticalTo(conn))

		Expect(component.Disconnect("port")).To(BeNil())
		Expect(component.GetConnection("port")).To(BeNil())
	})

})
