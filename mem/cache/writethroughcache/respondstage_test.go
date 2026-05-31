package writethroughcache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Respond Stage", func() {
	var (
		mockCtrl *gomock.Controller
		mw       *pipelineMW
		topPort  *MockPort
		s        *respondStage
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("TopPort")).
			AnyTimes()

		mw = &pipelineMW{
			topPort: topPort,
		}
		mw.comp = modeling.NewBuilder[Spec, State, modeling.None]().
			WithEngine(nil).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{}).
			Build("Cache")

		s = &respondStage{cache: mw}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("read", func() {
		var readMeta messaging.MsgMeta

		BeforeEach(func() {
			next := &mw.comp.State

			readMeta = messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          "SomeSrc",
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x100,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
		})

		It("should stall if cannot send to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&messaging.SendError{})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.RspTo).To(Equal(readMeta.ID))
					Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
				})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.Transactions[0].Removed).To(BeTrue())
		})
	})

	Context("write", func() {
		var writeMeta messaging.MsgMeta

		BeforeEach(func() {
			next := &mw.comp.State

			writeMeta = messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          "SomeSrc",
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x100,
					WritePID:     1,
				},
			)
		})

		It("should stall if cannot send to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&messaging.SendError{})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(writeMeta.ID))
				})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.Transactions[0].Removed).To(BeTrue())
		})
	})

})
