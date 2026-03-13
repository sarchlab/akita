package simplecache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
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
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		mw = &pipelineMW{
			topPort: topPort,
		}
		mw.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{}).
			Build("Cache")

		s = &respondStage{cache: mw}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("read", func() {
		var readMeta sim.MsgMeta

		BeforeEach(func() {
			next := mw.comp.GetNextState()

			readMeta = sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.NumTransactions = 1
		})

		It("should stall if cannot send to top", func() {
			next := mw.comp.GetNextState()
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			next := mw.comp.GetNextState()
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.RspTo).To(Equal(readMeta.ID))
					Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
				})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.NumTransactions).To(Equal(0))
		})
	})

	Context("write", func() {
		var writeMeta sim.MsgMeta

		BeforeEach(func() {
			next := mw.comp.GetNextState()

			writeMeta = sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.NumTransactions = 1
		})

		It("should stall if cannot send to top", func() {
			next := mw.comp.GetNextState()
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			next := mw.comp.GetNextState()
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(writeMeta.ID))
				})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.NumTransactions).To(Equal(0))
		})
	})

})
