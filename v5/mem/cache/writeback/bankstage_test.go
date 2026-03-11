package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Bank Stage", func() {
	var (
		mockCtrl *gomock.Controller
		m        *middleware
		bs       *bankStage
		storage  *mem.Storage
		topPort  *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		storage = mem.NewStorage(4 * mem.KB)

		initialState := State{
			DirToBankBufIndices:             []bankBufState{{Indices: nil}},
			WriteBufferToBankBufIndices:     []bankBufState{{Indices: nil}},
			BankPipelineStages:              []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices:      []bankPostBufState{{Indices: nil}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &middleware{
			storage:      storage,
			topPort:      topPort,
			evictingList: make(map[uint64]bool),
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(sim.NewSerialEngine()).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				BankLatency:      10,
				Log2BlockSize:    6,
				WayAssociativity: 4,
				NumSets:          64,
				NumBanks:         1,
				NumReqPerCycle:   4,
			}).
			Build("Cache")

		m.comp.SetState(initialState)
		next := m.comp.GetNextState()

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		m.dirToBankBuffers = []*stateTransBuffer{{
			name:     "Cache.DirToBankBuf0",
			readItems:  &next.DirToBankBufIndices[0].Indices,
			writeItems: &next.DirToBankBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}
		m.writeBufferToBankBuffers = []*stateTransBuffer{{
			name:     "Cache.WBToBankBuf0",
			readItems:  &next.WriteBufferToBankBufIndices[0].Indices,
			writeItems: &next.WriteBufferToBankBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}
		m.mshrStageBuffer = &stateTransBuffer{
			name:     "Cache.MSHRStageBuf",
			readItems:  &next.MSHRStageBufEntries,
			writeItems: &next.MSHRStageBufEntries,
			capacity: 4,
			mw:       m,
		}
		m.writeBufferBuffer = &stateTransBuffer{
			name:     "Cache.WriteBufferBuf",
			readItems:  &next.WriteBufferBufIndices,
			writeItems: &next.WriteBufferBufIndices,
			capacity: 4,
			mw:       m,
		}
		m.bankPostBufAdapters = []*stateBankPostBufAdapter{{
			name:     "Cache.BankPostBuf0",
			readItems:  &next.BankPostPipelineBufIndices[0].Indices,
			writeItems: &next.BankPostPipelineBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}

		bs = &bankStage{
			cache:         m,
			bankID:        0,
			pipelineWidth: 4,
		}
		m.bankStages = []*bankStage{bs}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("completing a read hit transaction", func() {
		var (
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			next := m.comp.GetNextState()
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1

			storage.Write(0x40, []byte{1, 2, 3, 4, 5, 6, 7, 8})

			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x104
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "mem.ReadReq"
			trans = &transactionState{
				read:       read,
				blockSetID: 0,
				blockWayID: 0,
				hasBlock:   true,
				action:     bankReadHit,
			}
			m.inFlightTransactions = []*transactionState{trans}

			// Put transaction in bank post-pipeline buffer
			next.BankPostPipelineBufIndices[0].Indices = []int{0}
			bs.inflightTransCount = 1
		})

		It("should stall if send buffer is full", func() {
			topPort.EXPECT().CanSend().Return(false).AnyTimes()

			m.syncForTest()

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
		})

		It("should read and send response", func() {
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.Meta().RspTo).To(Equal(read.ID))
					Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				})

			m.syncForTest()

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			next := m.comp.GetNextState()
			block := &next.DirectoryState.Sets[0].Blocks[0]
			Expect(block.ReadCount).To(Equal(0))
			Expect(m.inFlightTransactions).
				NotTo(ContainElement(trans))
			Expect(bs.inflightTransCount).To(Equal(0))
		})
	})

	Context("completing a write-hit transaction", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
		)

		BeforeEach(func() {
			next := m.comp.GetNextState()
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1
			block.IsLocked = true

			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x104
			write.Data = []byte{5, 6, 7, 8}
			write.TrafficBytes = len([]byte{5, 6, 7, 8}) + 12
			write.TrafficClass = "mem.WriteReq"
			trans = &transactionState{
				write:      write,
				blockSetID: 0,
				blockWayID: 0,
				hasBlock:   true,
				action:     bankWriteHit,
			}
			m.inFlightTransactions = []*transactionState{trans}
			next.BankPostPipelineBufIndices[0].Indices = []int{0}
			bs.inflightTransCount = 1
		})

		It("should stall if send buffer is full", func() {
			topPort.EXPECT().CanSend().Return(false).AnyTimes()

			m.syncForTest()

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
		})

		It("should write and send response", func() {
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(write.ID))
				})

			m.syncForTest()

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			data, _ := storage.Read(0x44, 4)
			Expect(data).To(Equal([]byte{5, 6, 7, 8}))
			next := m.comp.GetNextState()
			block := &next.DirectoryState.Sets[0].Blocks[0]
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsDirty).To(BeTrue())
			Expect(m.inFlightTransactions).
				NotTo(ContainElement(trans))
			Expect(bs.inflightTransCount).To(Equal(0))
		})
	})

	Context("completing a write fetched transaction", func() {
		var (
			trans *transactionState
		)

		BeforeEach(func() {
			next := m.comp.GetNextState()
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.IsLocked = true

			fetchedData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}

			trans = &transactionState{
				blockSetID:       0,
				blockWayID:       0,
				hasBlock:         true,
				mshrData:         fetchedData,
				mshrTransactions: []*transactionState{},
				action:           bankWriteFetched,
			}
			m.inFlightTransactions = []*transactionState{trans}
			next.BankPostPipelineBufIndices[0].Indices = []int{0}
			bs.inflightTransCount = 1
		})

		It("should write to storage and send to mshr stage", func() {
			m.syncForTest()

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			writtenData, _ := storage.Read(0x40, 64)
			Expect(writtenData).To(Equal(trans.mshrData))
			next := m.comp.GetNextState()
			block := &next.DirectoryState.Sets[0].Blocks[0]
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsValid).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(0))
		})
	})

	Context("finalizing a read for eviction action", func() {
		var (
			trans *transactionState
		)

		BeforeEach(func() {
			trans = &transactionState{
				hasVictim:          true,
				victimTag:          0x200,
				victimCacheAddress: 0x300,
				victimDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
				action:       bankEvictAndFetch,
				evictingAddr: 0x200,
			}
			m.inFlightTransactions = []*transactionState{trans}
			next := m.comp.GetNextState()
			next.BankPostPipelineBufIndices[0].Indices = []int{0}
			bs.inflightTransCount = 1
		})

		It("should send eviction to write buffer", func() {
			data := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			storage.Write(0x300, data)

			m.syncForTest()

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			Expect(trans.action).To(Equal(writeBufferEvictAndFetch))
			Expect(trans.evictingData).To(Equal(data))
			Expect(bs.inflightTransCount).To(Equal(0))
		})
	})
})
