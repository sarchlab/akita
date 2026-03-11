package arbitration

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("XBar", func() {
	var (
		mockCtrl         *gomock.Controller
		buf1, buf1Remote *MockBuffer
		buf2             *MockBuffer
		buf3, buf3Remote *MockBuffer
		buf4, buf4Remote *MockBuffer
		xbar             *xbarArbiter
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		buf1 = NewMockBuffer(mockCtrl)
		buf2 = NewMockBuffer(mockCtrl)
		buf3 = NewMockBuffer(mockCtrl)
		buf4 = NewMockBuffer(mockCtrl)
		buf1Remote = NewMockBuffer(mockCtrl)
		buf3Remote = NewMockBuffer(mockCtrl)
		buf4Remote = NewMockBuffer(mockCtrl)

		xbar = NewXBarArbiter().(*xbarArbiter)
		xbar.AddBuffer(buf1)
		xbar.AddBuffer(buf2)
		xbar.AddBuffer(buf3)
		xbar.AddBuffer(buf4)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should arbitrate", func() {
		msg := &sim.MsgMeta{
			ID: sim.GetIDGenerator().Generate(),
		}
		flit1 := &messaging.Flit{}
		flit1.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit1.TrafficClass = reflect.TypeOf(msg).String()
		flit1.Msg = msg
		flit1.OutputBuf = buf1Remote
		flit2 := &messaging.Flit{}
		flit2.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit2.TrafficClass = reflect.TypeOf(msg).String()
		flit2.Msg = msg
		flit2.OutputBuf = buf1Remote
		flit3 := &messaging.Flit{}
		flit3.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit3.TrafficClass = reflect.TypeOf(msg).String()
		flit3.Msg = msg
		flit3.OutputBuf = buf3Remote
		flit4 := &messaging.Flit{}
		flit4.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit4.TrafficClass = reflect.TypeOf(msg).String()
		flit4.Msg = msg
		flit4.OutputBuf = buf4Remote
		flit5 := &messaging.Flit{}
		flit5.ID = fmt.Sprintf("flit-%d-msg-%s-%s", 0, msg.Meta().ID, sim.GetIDGenerator().Generate())
		flit5.TrafficClass = reflect.TypeOf(msg).String()
		flit5.Msg = msg
		flit5.OutputBuf = buf1Remote

		buf1.EXPECT().Peek().Return(flit1)
		buf2.EXPECT().Peek().Return(flit2)
		buf3.EXPECT().Peek().Return(flit3)
		buf4.EXPECT().Peek().Return(flit4)

		bufs := xbar.Arbitrate()
		Expect(bufs).To(HaveLen(3))
		Expect(bufs[0]).To(BeIdenticalTo(buf1))
		Expect(bufs[1]).To(BeIdenticalTo(buf3))
		Expect(bufs[2]).To(BeIdenticalTo(buf4))

		buf1.EXPECT().Peek().Return(flit5)
		buf2.EXPECT().Peek().Return(flit2)
		buf3.EXPECT().Peek().Return(nil)
		buf4.EXPECT().Peek().Return(nil)

		bufs = xbar.Arbitrate()
		Expect(bufs).To(HaveLen(1))
		Expect(bufs[0]).To(BeIdenticalTo(buf2))
	})
})
