package sim

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type sampleMsg struct {
	MsgMeta
}

func (m *sampleMsg) Meta() *MsgMeta {
	return &m.MsgMeta
}

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

	It("should send successfully", func() {
		msg := &sampleMsg{}
		conn.EXPECT().Send(msg).Return(nil)

		err := port.Send(msg)

		Expect(err).To(BeNil())
	})

	It("should propagate error when sending is not successful", func() {
		msg := &sampleMsg{}
		err := &SendError{}
		conn.EXPECT().Send(msg).Return(err)

		errRet := port.Send(msg)

		Expect(errRet).NotTo(BeNil())
	})

	It("should recv when successful", func() {
		msg := &sampleMsg{}
		msg.RecvTime = 10

		comp.EXPECT().NotifyRecv(VTimeInSec(10), port)

		errRet := port.Recv(msg)

		Expect(errRet).To(BeNil())
	})

	It("should fail to receive when buffer is full", func() {
		msg := &sampleMsg{}
		msg.RecvTime = 10
		port.buf = NewBuffer("Buf", 4)
		port.buf.Push(msg)
		port.buf.Push(msg)
		port.buf.Push(msg)
		port.buf.Push(msg)

		errRet := port.Recv(msg)

		Expect(errRet).NotTo(BeNil())
	})

	It("should return nil when peeking empty port", func() {
		msg := port.Peek()

		Expect(msg).To(BeNil())
	})

	It("should allow component to peek message", func() {
		msg := &sampleMsg{}
		port.buf.Push(msg)

		msgRet := port.Peek()

		Expect(msgRet).To(BeIdenticalTo(msg))
	})

	It("should return nil when retrieving empty port", func() {
		msg := port.Retrieve(10)

		Expect(msg).To(BeNil())
	})

	It("should allow component to retrieve message", func() {
		msg := &sampleMsg{}
		port.buf.Push(msg)

		msgRet := port.Retrieve(10)

		Expect(msgRet).To(BeIdenticalTo(msg))
	})
})
