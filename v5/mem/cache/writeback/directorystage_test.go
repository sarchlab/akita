package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("DirectoryStage", func() {
	var (
		mockCtrl *gomock.Controller
		ds       *directoryStage
		m        *pipelineMW
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		initialState := State{
			DirToBankBufIndices:             []bankBufState{{Indices: nil}},
			WriteBufferToBankBufIndices:     []bankBufState{{Indices: nil}},
			BankPipelineStages:              []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices:      []bankPostBufState{{Indices: nil}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &pipelineMW{
			evictingList: make(map[uint64]bool),
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumMSHREntry:     16,
				NumSets:          64,
				NumBanks:         1,
			}).
			Build("Cache")

		m.comp.SetState(initialState)
		next := m.comp.GetNextState()

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		m.dirStageBuffer = &stateTransBuffer{
			name:     "Cache.DirStageBuf",
			readItems:  &next.DirStageBufIndices,
			writeItems: &next.DirStageBufIndices,
			capacity: 4,
			mw:       m,
		}
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
		m.writeBufferBuffer = &stateTransBuffer{
			name:     "Cache.WriteBufferBuf",
			readItems:  &next.WriteBufferBufIndices,
			writeItems: &next.WriteBufferBufIndices,
			capacity: 4,
			mw:       m,
		}
		m.dirPostBufAdapter = &stateDirPostBufAdapter{
			name:     "Cache.DirPostBuf",
			readItems:  &next.DirPostPipelineBufIndices,
			writeItems: &next.DirPostPipelineBufIndices,
			capacity: 4,
			mw:       m,
		}

		ds = &directoryStage{
			cache: m,
		}
		m.dirStage = ds
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
			read.AccessByteSize = 64
			read.TrafficBytes = 12
			read.TrafficClass = "mem.ReadReq"
			trans = &transactionState{
				read: read,
			}
			m.inFlightTransactions = []*transactionState{trans}

			// Put trans in dir post pipeline buf
			next := m.comp.GetNextState()
			next.DirPostPipelineBufIndices = []int{0}
		})

		Context("mshr hit", func() {
			BeforeEach(func() {
				next := m.comp.GetNextState()
				cache.MSHRAdd(&next.MSHRState, 16, vm.PID(1), 0x100)
			})

			It("should add to MSHR", func() {
				m.syncForTest()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := m.comp.GetNextState()
				Expect(next.MSHRState.Entries[0].TransactionIndices).To(HaveLen(1))
			})
		})

		Context("hit", func() {
			BeforeEach(func() {
				next := m.comp.GetNextState()
				setID := int(0x100 / uint64(64) % uint64(64))
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			})

			It("should pass transaction to bank", func() {
				m.syncForTest()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := m.comp.GetNextState()
				setID := int(0x100 / uint64(64) % uint64(64))
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				Expect(block.ReadCount).To(Equal(1))
				Expect(trans.action).To(Equal(bankReadHit))
			})
		})

		Context("miss, mshr miss, no need to evict", func() {
			It("should create mshr entry and fetch", func() {
				m.syncForTest()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := m.comp.GetNextState()
				Expect(next.MSHRState.Entries).To(HaveLen(1))
			})
		})

		Context("miss, mshr miss, need eviction", func() {
			BeforeEach(func() {
				next := m.comp.GetNextState()
				setID := int(0x100 / uint64(64) % uint64(64))
				for i := range 4 {
					block := &next.DirectoryState.Sets[setID].Blocks[i]
					block.PID = 2
					block.Tag = uint64(0x200 + i*0x1000)
					block.CacheAddress = uint64(i * 64)
					block.IsValid = true
					block.IsDirty = true
				}
			})

			It("should do evict", func() {
				m.syncForTest()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankEvictAndFetch))
			})
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
			write.TrafficClass = "mem.WriteReq"
			trans = &transactionState{
				write: write,
			}
			m.inFlightTransactions = []*transactionState{trans}

			next := m.comp.GetNextState()
			next.DirPostPipelineBufIndices = []int{0}
		})

		Context("hit", func() {
			BeforeEach(func() {
				next := m.comp.GetNextState()
				setID := int(0x100 / uint64(64) % uint64(64))
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			})

			It("should send to bank", func() {
				m.syncForTest()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankWriteHit))
			})
		})

		Context("miss, write full line, no eviction", func() {
			BeforeEach(func() {
				write.Data = make([]byte, 64)
			})

			It("should send to bank", func() {
				m.syncForTest()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankWriteHit))
			})
		})
	})
})
