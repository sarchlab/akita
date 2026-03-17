package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

func newTestMsg() *MsgMeta {
	return &MsgMeta{
		ID: GetIDGenerator().Generate(),
	}
}

var _ = Describe("DefaultPort", func() {
	var (
		mockController *gomock.Controller
		comp           *MockComponent
		conn           *MockConnection
		port           *defaultPort
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		comp = NewMockComponent(mockController)
		conn = NewMockConnection(mockController)
		port = NewPort(comp, 4, 4, "Port").(*defaultPort)
		port.SetConnection(conn)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should return component", func() {
		Expect(port.Component()).To(BeIdenticalTo(comp))
	})

	It("should return name", func() {
		Expect(port.Name()).To(Equal("Port"))
	})

	It("should set connection", func() {
		Expect(port.conn).To(BeIdenticalTo(conn))
	})

	It("should be panic if port is not msg src", func() {
		msg := newTestMsg()

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should be panic if msg dst is not set", func() {
		msg := newTestMsg()
		msg.Src = port.AsRemote()

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should be panic if msg src is the same as dst", func() {
		msg := newTestMsg()
		msg.Src = port.AsRemote()
		msg.Dst = port.AsRemote()

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should send successfully", func() {
		dst := NewPort(comp, 4, 4, "DstPort")
		msg := newTestMsg()
		msg.Src = port.AsRemote()
		msg.Dst = dst.AsRemote()
		conn.EXPECT().NotifySend()

		err := port.Send(msg)

		Expect(err).To(BeNil())
		Expect(port.PeekOutgoing()).To(BeIdenticalTo(msg))
	})

	It("should propagate error when outgoing buff is full", func() {
		dst := NewPort(comp, 4, 4, "DstPort")
		msg := newTestMsg()
		msg.Src = port.AsRemote()
		msg.Dst = dst.AsRemote()

		port.outgoingBuf.Push(msg)
		port.outgoingBuf.Push(msg)
		port.outgoingBuf.Push(msg)
		port.outgoingBuf.Push(msg)

		errRet := port.Send(msg)

		Expect(errRet).NotTo(BeNil())
	})

	It("should deliver when successful", func() {
		msg := newTestMsg()

		comp.EXPECT().NotifyRecv(port)

		errRet := port.Deliver(msg)

		Expect(errRet).To(BeNil())
	})

	It("should fail to deliver when incoming buffer is full", func() {
		msg := newTestMsg()
		port.incomingBuf = newPortBuffer(4)
		port.incomingBuf.Push(msg)
		port.incomingBuf.Push(msg)
		port.incomingBuf.Push(msg)
		port.incomingBuf.Push(msg)

		errRet := port.Deliver(msg)

		Expect(errRet).NotTo(BeNil())
	})

	It("should return nil when peeking empty incoming buffer", func() {
		msg := port.PeekIncoming()

		Expect(msg).To(BeNil())
	})

	It("should allow component to peek message from incoming buffer", func() {
		msg := newTestMsg()
		port.incomingBuf.Push(msg)

		msgRet := port.PeekIncoming()

		Expect(msgRet).To(BeIdenticalTo(msg))
	})

	It("should return nil when peeking empty outgoing buffer", func() {
		msg := port.PeekOutgoing()

		Expect(msg).To(BeNil())
	})

	It("should allow component to peek message from outgoing buffer", func() {
		msg := newTestMsg()
		port.outgoingBuf.Push(msg)

		msgRet := port.PeekOutgoing()

		Expect(msgRet).To(BeIdenticalTo(msg))
	})

	It("should return nil when retrieving empty incoming buffer", func() {
		msg := port.RetrieveIncoming()

		Expect(msg).To(BeNil())
	})

	It("should allow component to retrieve message from incoming buffer",
		func() {
			msg := newTestMsg()
			port.incomingBuf.Push(msg)

			msgRet := port.RetrieveIncoming()

			Expect(msgRet).To(BeIdenticalTo(msg))
		})

	It("should return nil when retrieving empty outgoing buffer", func() {
		msg := port.RetrieveOutgoing()

		Expect(msg).To(BeNil())
	})

	It("should allow component to retrieve message from outgoing buffer",
		func() {
			msg := newTestMsg()
			port.outgoingBuf.Push(msg)

			msgRet := port.RetrieveOutgoing()

			Expect(msgRet).To(BeIdenticalTo(msg))
		})

	It("should return 0 for NumIncoming when buffer is empty", func() {
		Expect(port.NumIncoming()).To(Equal(0))
	})

	It("should return 0 for NumOutgoing when buffer is empty", func() {
		Expect(port.NumOutgoing()).To(Equal(0))
	})

	It("should return correct NumIncoming after delivering messages", func() {
		msg := newTestMsg()
		comp.EXPECT().NotifyRecv(port)
		port.Deliver(msg)

		Expect(port.NumIncoming()).To(Equal(1))
	})

	It("should return correct NumOutgoing after pushing messages", func() {
		msg := newTestMsg()
		port.outgoingBuf.Push(msg)
		port.outgoingBuf.Push(msg)

		Expect(port.NumOutgoing()).To(Equal(2))
	})
})
