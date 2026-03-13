package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
	"go.uber.org/mock/gomock"
)

var _ = Describe("WriteBufferStage", func() {
	var (
		mockCtrl   *gomock.Controller
		m          *pipelineMW
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
			DirStageBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirStageBuf", Cap: 4,
			},
			DirToBankBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.DirToBankBuf", Cap: 4,
			}},
			WriteBufferToBankBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.WBToBankBuf", Cap: 4,
			}},
			MSHRStageBuf: stateutil.Buffer[int]{
				BufferName: "Cache.MSHRStageBuf", Cap: 4,
			},
			WriteBufferBuf: stateutil.Buffer[int]{
				BufferName: "Cache.WriteBufferBuf", Cap: 4,
			},
			DirPipeline: stateutil.Pipeline[int]{Width: 4, NumStages: 0},
			DirPostPipelineBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirPostBuf", Cap: 4,
			},
			BankPipelines: []stateutil.Pipeline[int]{{Width: 4, NumStages: 10}},
			BankPostPipelineBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.BankPostBuf", Cap: 4,
			}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &pipelineMW{
			bottomPort:   bottomPort,
			evictingList: make(map[uint64]bool),
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:     6,
				NumReqPerCycle:    4,
				WayAssociativity:  4,
				NumSets:           64,
				NumBanks:          1,
				AddressMapperType: "single",
				RemotePortNames:   []string{"DRAM"},
			}).
			Build("Cache")

		m.comp.SetState(initialState)

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

		m.syncForTest()

		ret := wb.Tick()

		Expect(ret).To(BeFalse())
	})

	Context("processing new writeBufferFetch transactions", func() {
		It("should fetch from bottom", func() {
			read := &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := &transactionState{
				Action:       writeBufferFetch,
				FetchAddress: 0x100,
				FetchPID:     1,
				BlockSetID:   0,
				BlockWayID:   0,
				HasBlock:     true,
				HasRead:      true,
				ReadMeta:     read.MsgMeta,
				ReadAddress:  0x100,
			}
			m.inFlightTransactions = []*transactionState{trans}

			next := m.comp.GetNextState()
			next.WriteBufferBuf.Elements = []int{0}

			bottomPort.EXPECT().PeekIncoming().Return(nil)
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().Send(gomock.Any())

			m.syncForTest()

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
				Action:       writeBufferFetch,
				FetchAddress: 0x100,
				HasRead:      true,
				ReadMeta:     read.MsgMeta,
				ReadAddress:  0x100,
			}
			m.inFlightTransactions = []*transactionState{trans}

			next := m.comp.GetNextState()
			next.WriteBufferBuf.Elements = []int{0}

			bottomPort.EXPECT().PeekIncoming().Return(nil)

			m.syncForTest()

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
				EvictingAddr: 0x200,
				EvictingPID:  2,
				EvictingData: make([]byte, 64),
				HasRead:      true,
				ReadMeta:     read.MsgMeta,
				ReadAddress:  0x200,
			}
			m.inFlightTransactions = []*transactionState{trans}
			wb.pendingEvictions = []*transactionState{trans}

			bottomPort.EXPECT().PeekIncoming().Return(nil)
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().Send(gomock.Any())

			m.syncForTest()

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.pendingEvictions).To(HaveLen(0))
			Expect(wb.inflightEviction).To(HaveLen(1))
		})

		It("should stall if too many inflight evictions", func() {
			wb.inflightEviction = make([]*transactionState, 4)
			wb.pendingEvictions = []*transactionState{{}}

			bottomPort.EXPECT().PeekIncoming().Return(nil)

			m.syncForTest()

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
				HasEvictionWriteReq:  true,
				EvictionWriteReqMeta: evictWrite.MsgMeta,
				HasRead:              true,
				ReadMeta:             read.MsgMeta,
				ReadAddress:          0,
			}
			m.inFlightTransactions = []*transactionState{trans}
			wb.inflightEviction = []*transactionState{trans}

			rsp := &mem.WriteDoneRsp{}
			rsp.RspTo = "write-1"
			rsp.TrafficClass = "mem.WriteDoneRsp"

			bottomPort.EXPECT().PeekIncoming().Return(rsp)
			bottomPort.EXPECT().RetrieveIncoming().Return(rsp)

			m.syncForTest()

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.inflightEviction).To(HaveLen(0))
		})
	})
})
