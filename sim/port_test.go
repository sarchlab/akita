package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

type sampleMsg struct {
	MsgMeta
}

func NewSampleMsg() *sampleMsg {
	m := &sampleMsg{}
	return m
}

func (m *sampleMsg) Meta() *MsgMeta {
	return &m.MsgMeta
}

func (m *sampleMsg) Clone() Msg {
	cloneMsg := *m
	cloneMsg.ID = GetIDGenerator().Generate()

	return &cloneMsg
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
		msg := NewSampleMsg()

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should be panic if msg dst is not set", func() {
		msg := NewSampleMsg()
		msg.Src = port.AsRemote()

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should be panic if msg src is the same as dst", func() {
		msg := NewSampleMsg()
		msg.Src = port.AsRemote()
		msg.Dst = port.AsRemote()

		Expect(func() { port.Send(msg) }).To(Panic())
	})

	It("should send successfully", func() {
		dst := NewPort(comp, 4, 4, "DstPort")
		msg := &sampleMsg{}
		msg.Src = port.AsRemote()
		msg.Dst = dst.AsRemote()
		conn.EXPECT().NotifySend()

		err := port.Send(msg)

		Expect(err).To(BeNil())
		Expect(port.PeekOutgoing()).To(BeIdenticalTo(msg))
	})

	It("should propagate error when outgoing buff is full", func() {
		dst := NewPort(comp, 4, 4, "DstPort")
		msg := &sampleMsg{}
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
		msg := &sampleMsg{}

		comp.EXPECT().NotifyRecv(port)

		errRet := port.Deliver(msg)

		Expect(errRet).To(BeNil())
	})

	It("should fail to deliver when incoming buffer is full", func() {
		msg := &sampleMsg{}
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
		msg := port.RetrieveIncoming()

		Expect(msg).To(BeNil())
	})

	It("should allow component to retrieve message from incoming buffer",
		func() {
			msg := &sampleMsg{}
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
			msg := &sampleMsg{}
			port.outgoingBuf.Push(msg)

			msgRet := port.RetrieveOutgoing()

			Expect(msgRet).To(BeIdenticalTo(msg))
		})
})
