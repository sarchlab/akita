package sim

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LimitNumMsgPort", func() {
	var (
		mockController *gomock.Controller
		comp           *MockComponent
		conn           *MockConnection
		port           *LimitNumMsgPort
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		comp = NewMockComponent(mockController)
		conn = NewMockConnection(mockController)
		port = NewLimitNumMsgPort(comp, 4, "Port")
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
		msg := NewSampleMsg()

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should be panic if msg dst is nil", func() {
		msg := NewSampleMsg()
		msg.Src = port
		msg.Dst = nil

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should be panic if msg src is the same as dst", func() {
		msg := NewSampleMsg()
		msg.Src = port
		msg.Dst = port

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	FIt("should send successfully", func() {
		dst := NewLimitNumMsgPort(comp, 4, "Port")
		msg := &sampleMsg{}
		msg.SendTime = 10
		msg.Src = port
		msg.Dst = dst
		conn.EXPECT().NotifySend(msg.SendTime)

		err := port.Send(msg)

		Expect(err).To(BeNil())
		Expect(port.outgoingBuf).To(ContainElement(msg))
	})

	It("should propagate error when outgoing buff is full", func() {
		dst := NewLimitNumMsgPort(comp, 4, "Port")
		msg := &sampleMsg{}
		msg.Src = port
		msg.Dst = dst

		port.outgoingBuf.Push(msg)
		port.outgoingBuf.Push(msg)
		port.outgoingBuf.Push(msg)
		port.outgoingBuf.Push(msg)

		errRet := port.Send(msg)

		Expect(errRet).NotTo(BeNil())
	})

	It("should deliver when successful", func() {
		msg := &sampleMsg{}
		msg.RecvTime = 10

		comp.EXPECT().NotifyRecv(VTimeInSec(10), port)

		errRet := port.Deliver(msg)

		Expect(errRet).To(BeNil())
	})

	It("should fail to deliver when incoming buffer is full", func() {
		msg := &sampleMsg{}
		msg.RecvTime = 10
		port.incomingBuf = NewBuffer("Buf", 4)
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
		msg := &sampleMsg{}
		port.incomingBuf.Push(msg)

		msgRet := port.PeekIncoming()

		Expect(msgRet).To(BeIdenticalTo(msg))
	})

	It("should return nil when peeking empty outgoing buffer", func() {
		msg := port.PeekOutgoing()

		Expect(msg).To(BeNil())
	})

	It("should allow component to peek message from outgoing buffer", func() {
		msg := &sampleMsg{}
		port.outgoingBuf.Push(msg)

		msgRet := port.PeekOutgoing()

		Expect(msgRet).To(BeIdenticalTo(msg))
	})

	It("should return nil when retrieving empty incoming buffer", func() {
		msg := port.RetrieveIncoming(10)

		Expect(msg).To(BeNil())
	})

	It("should allow component to retrieve message from incoming buffer", func() {
		msg := &sampleMsg{}
		port.incomingBuf.Push(msg)

		msgRet := port.RetrieveIncoming(10)

		Expect(msgRet).To(BeIdenticalTo(msg))
	})

	It("should return nil when retrieving empty outgoing buffer", func() {
		msg := port.RetrieveOutgoing()

		Expect(msg).To(BeNil())
	})

	It("should allow component to retrieve message from outgoing buffer", func() {
		msg := &sampleMsg{}
		port.outgoingBuf.Push(msg)

		msgRet := port.RetrieveOutgoing()

		Expect(msgRet).To(BeIdenticalTo(msg))
	})
})
