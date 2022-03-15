package sim

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BufferedSender", func() {
	var (
		mockCtrl *gomock.Controller
		port     *MockPort
		buffer   *MockBuffer
		sender   BufferedSender
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		port = NewMockPort(mockCtrl)
		buffer = NewMockBuffer(mockCtrl)
		buffer.EXPECT().Capacity().Return(2).AnyTimes()
		sender = NewBufferedSender(port, buffer)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should panic if to send messages more than capacity", func() {
		Expect(func() { sender.CanSend(3) }).To(Panic())
	})

	It("should buffer", func() {
		buffer.EXPECT().Size().Return(0)
		Expect(sender.CanSend(1)).To(BeTrue())
		buffer.EXPECT().Size().Return(0)
		Expect(sender.CanSend(2)).To(BeTrue())

		buffer.EXPECT().Size().Return(1)
		Expect(sender.CanSend(1)).To(BeTrue())
		buffer.EXPECT().Size().Return(1)
		Expect(sender.CanSend(2)).To(BeFalse())

		buffer.EXPECT().Size().Return(2)
		Expect(sender.CanSend(1)).To(BeFalse())
		buffer.EXPECT().Size().Return(2)
		Expect(sender.CanSend(2)).To(BeFalse())
	})

	It("should send", func() {
		msg1 := &sampleMsg{}
		buffer.EXPECT().Push(msg1)
		sender.Send(msg1)

		msg2 := &sampleMsg{}
		buffer.EXPECT().Push(msg2)
		sender.Send(msg2)

		port.EXPECT().Send(msg1)
		buffer.EXPECT().Peek().Return(msg1)
		buffer.EXPECT().Pop()
		sent := sender.Tick(10)
		Expect(msg1.Meta().SendTime).To(Equal(VTimeInSec(10)))
		Expect(sent).To(BeTrue())

		port.EXPECT().Send(msg2)
		buffer.EXPECT().Peek().Return(msg2)
		buffer.EXPECT().Pop()
		sent = sender.Tick(11)
		Expect(sent).To(BeTrue())
		Expect(msg2.Meta().SendTime).To(Equal(VTimeInSec(11)))
	})

	It("should clear", func() {
		buffer.EXPECT().Clear()
		sender.Clear()
	})

	It("should do nothing if buffer is empty", func() {
		buffer.EXPECT().Peek().Return(nil)
		sent := sender.Tick(10)
		Expect(sent).To(BeFalse())
	})

	It("should do nothing if send failed", func() {
		msg1 := &sampleMsg{}
		buffer.EXPECT().Peek().Return(msg1)
		port.EXPECT().Send(msg1).Return(NewSendError())

		sent := sender.Tick(10)

		Expect(sent).To(BeFalse())
	})

})
