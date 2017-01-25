package requestsys_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/syifan/yaotsu/requestsys"
)

type mockConn struct {
	*requestsys.BasicConn

	canSend    bool
	canReceive bool
}

func newMockConn() *mockConn {
	return &mockConn{requestsys.NewBasicConn(), false, false}
}

func (c *mockConn) CanSend(req *requestsys.Request) bool {
	return c.canSend
}

func (c *mockConn) CanReceive(req *requestsys.Request) bool {
	return c.canReceive
}

func (c *mockConn) Send(req *requestsys.Request) error {
	return nil
}

func (c *mockConn) Receive(req *requestsys.Request) error {
	return nil
}

type mockComponent struct {
	*requestsys.BasicComponent

	canProcess bool
}

func newMockComponent() *mockComponent {
	return &mockComponent{requestsys.NewBasicComponent("mock"), false}
}

var _ = Describe("DirectConnection", func() {

	Context("The connection and disconnection", func() {
		It("should not connect twice", func() {
			socket := new(requestsys.Socket)
			conn := newMockConn()

			err := socket.Connect(conn)
			Expect(err).To(BeNil())

			// Connecting again means disconnect first and connect
			err = socket.Connect(conn)
			Expect(err).To(BeNil())
		})

		It("should only disconnect once", func() {
			socket := new(requestsys.Socket)
			conn := newMockConn()
			_ = socket.Connect(conn)

			err := socket.Disconnect()
			Expect(err).To(BeNil())

			err = socket.Disconnect()
			Expect(err).ToNot(BeNil())
		})
	})

	Context("when not connected", func() {
		It("should not allow sending", func() {
			socket := new(requestsys.Socket)
			Expect(socket.CanSend(nil)).To(Equal(false))
		})

		It("sending should return error", func() {
			socket := new(requestsys.Socket)
			Expect(socket.Send(nil)).NotTo(BeNil())
		})

		It("should not allow receiving", func() {
			socket := new(requestsys.Socket)
			Expect(socket.CanReceive(nil)).To(Equal(false))
		})

		It("receiving should return error", func() {
			socket := new(requestsys.Socket)
			Expect(socket.Receive(nil)).NotTo(BeNil())
		})
	})

	Context("when connected", func() {
		socket := new(requestsys.Socket)
		component := newMockComponent()
		requestsys.BindSocket(component, socket)
		It("should check the connection for can or cannot send", func() {

		})
	})
})
