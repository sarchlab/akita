package conn_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/conn/mock_conn"
)

var _ = Describe("BasicComponent", func() {

	var (
		mockCtrl   *gomock.Controller
		component  *conn.BasicComponent
		connection *mock_conn.MockConnection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		component = conn.NewBasicComponent("test_comp")
		connection = mock_conn.NewMockConnection(mockCtrl)
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

		component.Connect("port", connection)

		Expect(component.GetConnection("port")).To(BeIdenticalTo(connection))

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

		component.Connect("port", connection)
		Expect(component.GetConnection("port")).To(BeIdenticalTo(connection))

		Expect(component.Disconnect("port")).To(BeNil())
		Expect(component.GetConnection("port")).To(BeNil())
	})

})
