package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MSHR Stage", func() {
	var (
		mockCtrl *gomock.Controller
		m        *middleware
		ms       *mshrStage
		topPort  *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		initialState := State{
			DirToBankBufIndices:             []bankBufState{{Indices: nil}},
			WriteBufferToBankBufIndices:     []bankBufState{{Indices: nil}},
			BankPipelineStages:              []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices:      []bankPostBufState{{Indices: nil}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &middleware{
			topPort:      topPort,
			evictingList: make(map[uint64]bool),
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:  6,
				NumReqPerCycle: 4,
			}).
			Build("Cache")

		m.comp.SetState(initialState)
		next := m.comp.GetNextState()

		m.mshrStageBuffer = &stateTransBuffer{
			name:     "Cache.MSHRStageBuf",
			readItems:  &next.MSHRStageBufEntries,
			writeItems: &next.MSHRStageBufEntries,
			capacity: 4,
			mw:       m,
		}
		m.inFlightTransactions = nil

		ms = &mshrStage{
			cache: m,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no entry in input buffer", func() {
		m.syncForTest()

		ret := ms.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should stall if topSender is busy", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := &transactionState{read: read}

		mshrTrans := &transactionState{
			mshrTransactions: []*transactionState{trans},
			mshrData: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}

		m.inFlightTransactions = []*transactionState{trans, mshrTrans}

		// Push mshrTrans to the MSHR stage buffer
		next := m.comp.GetNextState()
		next.MSHRStageBufEntries = []int{1}

		topPort.EXPECT().CanSend().Return(false)

		m.syncForTest()

		ret := ms.Tick()

		Expect(ret).To(BeFalse())
		Expect(ms.hasProcessingTrans).To(BeTrue())
	})

	It("should send data ready to top", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := &transactionState{read: read}

		mshrTrans := &transactionState{
			mshrTransactions: []*transactionState{trans},
			mshrData: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}
		m.inFlightTransactions = []*transactionState{trans, mshrTrans}

		next := m.comp.GetNextState()
		next.MSHRStageBufEntries = []int{1}

		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		m.syncForTest()

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingTrans).To(BeFalse())
		Expect(m.inFlightTransactions).NotTo(ContainElement(trans))
	})

	It("should discard the request if it is no longer inflight", func() {
		staleTrans := &transactionState{}

		mshrTrans := &transactionState{
			mshrTransactions: []*transactionState{staleTrans},
			mshrData: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}

		m.inFlightTransactions = []*transactionState{mshrTrans}

		next := m.comp.GetNextState()
		next.MSHRStageBufEntries = []int{0}

		topPort.EXPECT().CanSend().Return(true)

		m.syncForTest()

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingTrans).To(BeFalse())
	})
})
