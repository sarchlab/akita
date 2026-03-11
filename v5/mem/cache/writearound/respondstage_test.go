package writearound

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
		cache    *Comp
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

		cache = &Comp{
			topPort: topPort,
		}
		cache.Component = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{}).
			Build("Cache")

		s = &respondStage{cache: cache}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("read", func() {
		var (
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x100
			read.PID = 1
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "req"
			trans = &transactionState{read: read}
			cache.transactions = append(cache.transactions, trans)
		})

		It("should stall if cannot send to top", func() {
			trans.data = []byte{1, 2, 3, 4}
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			trans.data = []byte{1, 2, 3, 4}
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.RspTo).To(Equal(read.ID))
					Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
				})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(cache.transactions).NotTo(ContainElement((trans)))
		})
	})

	Context("write", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
		)

		BeforeEach(func() {
			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x100
			write.PID = 1
			write.TrafficBytes = 12
			write.TrafficClass = "req"
			trans = &transactionState{write: write}
			cache.transactions = append(cache.transactions, trans)
		})

		It("should stall if cannot send to top", func() {
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			trans.data = []byte{1, 2, 3, 4}
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(write.ID))
				})

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(cache.transactions).NotTo(ContainElement((trans)))
		})
	})

})
