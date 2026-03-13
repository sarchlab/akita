package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
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
			trans *transactionState
		)

		BeforeEach(func() {
			read := &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x100
			read.PID = 1
			read.AccessByteSize = 64
			read.TrafficBytes = 12
			read.TrafficClass = "mem.ReadReq"
			trans = &transactionState{
				HasRead:            true,
				ReadMeta:           read.MsgMeta,
				ReadAddress:        read.Address,
				ReadAccessByteSize: read.AccessByteSize,
				ReadPID:            read.PID,
			}
			m.inFlightTransactions = []*transactionState{trans}

			// Put trans in dir post pipeline buf
			next := m.comp.GetNextState()
			next.DirPostPipelineBuf.Elements = []int{0}
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
				Expect(trans.Action).To(Equal(bankReadHit))
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
				Expect(trans.Action).To(Equal(bankEvictAndFetch))
			})
		})
	})

	Context("write", func() {
		var (
			trans *transactionState
		)

		BeforeEach(func() {
			write := &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x100
			write.PID = 1
			write.TrafficBytes = 12
			write.TrafficClass = "mem.WriteReq"
			trans = &transactionState{
				HasWrite:     true,
				WriteMeta:    write.MsgMeta,
				WriteAddress: write.Address,
				WritePID:     write.PID,
			}
			m.inFlightTransactions = []*transactionState{trans}

			next := m.comp.GetNextState()
			next.DirPostPipelineBuf.Elements = []int{0}
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
				Expect(trans.Action).To(Equal(bankWriteHit))
			})
		})

		Context("miss, write full line, no eviction", func() {
			BeforeEach(func() {
				trans.WriteData = make([]byte, 64)
			})

			It("should send to bank", func() {
				m.syncForTest()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.Action).To(Equal(bankWriteHit))
			})
		})
	})
})
