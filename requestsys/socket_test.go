package requestsys_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/requestsys"
)

type mockConn struct {
	*requestsys.BasicConn

	canSend bool
	sent    *requestsys.Request
}

func newMockConn() *mockConn {
	return &mockConn{requestsys.NewBasicConn(), false, nil}
}

func (c *mockConn) CanSend(req *requestsys.Request) bool {
	return c.canSend
}

func (c *mockConn) Send(req *requestsys.Request) error {
	c.sent = req
	return nil
}

type mockComponent struct {
	*requestsys.BasicComponent

	canProcess bool
	processed  *requestsys.Request
}

func newMockComponent() *mockComponent {
	return &mockComponent{requestsys.NewBasicComponent("mock"), false, nil}
}

func (c *mockComponent) CanProcess(req *requestsys.Request) bool {
	return c.canProcess
}

func (c *mockComponent) Process(req *requestsys.Request) error {
	c.processed = req
	return nil
}

var _ = Describe("DirectConnection", func() {

	Context("The connection and disconnection", func() {
		It("should not connect twice", func() {
			socket := requestsys.NewSimpleSocket("test_sock")
			conn := newMockConn()

			err := socket.Connect(conn)
			Expect(err).To(BeNil())

			// Connecting again means disconnect first and connect
			err = socket.Connect(conn)
			Expect(err).To(BeNil())
		})

		It("should only disconnect once", func() {
			socket := requestsys.NewSimpleSocket("test_sock")
			conn := newMockConn()
			_ = socket.Connect(conn)

			err := socket.Disconnect()
			Expect(err).To(BeNil())

			err = socket.Disconnect()
			Expect(err).ToNot(BeNil())
		})
	})

	Context("when not connected", func() {
		socket := requestsys.NewSimpleSocket("test_sock")

		It("should not allow sending", func() {
			Expect(socket.CanSend(nil)).To(Equal(false))
		})

		It("sending should return error", func() {
			Expect(socket.Send(nil)).NotTo(BeNil())
		})

		It("should not allow receiving", func() {
			Expect(socket.CanRecv(nil)).To(Equal(false))
		})

		It("receiving should return error", func() {
			Expect(socket.Recv(nil)).NotTo(BeNil())
		})
	})

	Context("when connected", func() {
		socket := requestsys.NewSimpleSocket("test_sock")

		component := newMockComponent()
		requestsys.BindSocket(component, socket)

		conn := newMockConn()
		socket.Connect(conn)

		It("should check the connection for can or cannot send", func() {
			conn.canSend = true
			Expect(socket.CanSend(nil)).To(BeTrue())

			conn.canSend = false
			Expect(socket.CanSend(nil)).To(BeFalse())
		})

		It("should check the component for can or cannot receive", func() {
			component.canProcess = true
			Expect(socket.CanRecv(nil)).To(BeTrue())

			component.canProcess = false
			Expect(socket.CanRecv(nil)).To(BeFalse())
		})

		It("should forward send request to the connection", func() {
			req := new(requestsys.Request)
			socket.Send(req)
			Expect(conn.sent).To(BeIdenticalTo(req))
		})

		It("should let the component to process the incomming requests", func() {
			req := new(requestsys.Request)
			socket.Recv(req)
			Expect(component.processed).To(BeIdenticalTo(req))
		})

	})
})
