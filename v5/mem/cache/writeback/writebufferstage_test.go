package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("WriteBufferStage", func() {
	var (
		mockCtrl   *gomock.Controller
		m          *middleware
		wb         *writeBufferStage
		bottomPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
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
			bottomPort:   bottomPort,
			evictingList: make(map[uint64]bool),
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumSets:          64,
				NumBanks:         1,
				AddressMapperType: "single",
				RemotePortNames:   []string{"DRAM"},
			}).
			Build("Cache")

		m.comp.SetState(initialState)
		next := m.comp.GetNextState()

		m.writeBufferBuffer = &stateTransBuffer{
			name:     "Cache.WriteBufferBuf",
			items:    &next.WriteBufferBufIndices,
			capacity: 4,
			mw:       m,
		}
		m.writeBufferToBankBuffers = []*stateTransBuffer{{
			name:     "Cache.WBToBankBuf0",
			items:    &next.WriteBufferToBankBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}
		m.dirToBankBuffers = []*stateTransBuffer{{
			name:     "Cache.DirToBankBuf0",
			items:    &next.DirToBankBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}

		wb = &writeBufferStage{
			cache:               m,
			writeBufferCapacity: 16,
			maxInflightFetch:    4,
			maxInflightEviction: 4,
		}
		m.writeBuffer = wb
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transactions", func() {
		bottomPort.EXPECT().PeekIncoming().Return(nil)

		ret := wb.Tick()

		Expect(ret).To(BeFalse())
	})

	Context("processing new writeBufferFetch transactions", func() {
		It("should fetch from bottom", func() {
			read := &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := &transactionState{
				action:       writeBufferFetch,
				fetchAddress: 0x100,
				fetchPID:     1,
				blockSetID:   0,
				blockWayID:   0,
				hasBlock:     true,
				read:         read,
			}
			m.inFlightTransactions = []*transactionState{trans}

			next := m.comp.GetNextState()
			next.WriteBufferBufIndices = []int{0}

			bottomPort.EXPECT().PeekIncoming().Return(nil)
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().Send(gomock.Any())

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.inflightFetch).To(HaveLen(1))
		})

		It("should stall fetch if too many inflight fetches", func() {
			wb.inflightFetch = make([]*transactionState, 4)

			read := &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := &transactionState{
				action:       writeBufferFetch,
				fetchAddress: 0x100,
				read:         read,
			}
			m.inFlightTransactions = []*transactionState{trans}

			next := m.comp.GetNextState()
			next.WriteBufferBufIndices = []int{0}

			bottomPort.EXPECT().PeekIncoming().Return(nil)

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
		})
	})

	Context("writing evictions", func() {
		It("should send eviction to bottom", func() {
			read := &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := &transactionState{
				evictingAddr: 0x200,
				evictingPID:  2,
				evictingData: make([]byte, 64),
				read:         read,
			}
			m.inFlightTransactions = []*transactionState{trans}
			wb.pendingEvictions = []*transactionState{trans}

			bottomPort.EXPECT().PeekIncoming().Return(nil)
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().Send(gomock.Any())

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.pendingEvictions).To(HaveLen(0))
			Expect(wb.inflightEviction).To(HaveLen(1))
		})

		It("should stall if too many inflight evictions", func() {
			wb.inflightEviction = make([]*transactionState, 4)
			wb.pendingEvictions = []*transactionState{{}}

			bottomPort.EXPECT().PeekIncoming().Return(nil)

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
		})
	})

	Context("processing responses", func() {
		It("should process write done response", func() {
			evictWrite := &mem.WriteReq{}
			evictWrite.ID = "write-1"
			evictWrite.TrafficClass = "mem.WriteReq"

			read := &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := &transactionState{
				evictionWriteReq: evictWrite,
				read:             read,
			}
			m.inFlightTransactions = []*transactionState{trans}
			wb.inflightEviction = []*transactionState{trans}

			rsp := &mem.WriteDoneRsp{}
			rsp.RspTo = "write-1"
			rsp.TrafficClass = "mem.WriteDoneRsp"

			bottomPort.EXPECT().PeekIncoming().Return(rsp)
			bottomPort.EXPECT().RetrieveIncoming().Return(rsp)

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.inflightEviction).To(HaveLen(0))
		})
	})
})
