package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
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
			CacheState:   int(cacheStateRunning),
			EvictingList: make(map[uint64]bool),
			DirStageBuf:  queueing.NewBuffer[int]("Cache.DirStageBuf", 4),
			DirToBankBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.DirToBankBuf", 4),
			},
			WriteBufferToBankBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.WBToBankBuf", 4),
			},
			MSHRStageBuf:       queueing.NewBuffer[int]("Cache.MSHRStageBuf", 4),
			WriteBufferBuf:     queueing.NewBuffer[int]("Cache.WriteBufferBuf", 4),
			DirPipeline:        queueing.NewPipeline[int](4, 0),
			DirPostPipelineBuf: queueing.NewBuffer[int]("Cache.DirPostBuf", 4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.NewPipeline[int](4, 10),
			},
			BankPostPipelineBufs: []postPipelineBuf{
				newPostPipelineBuf(4),
			},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &pipelineMW{}
		m.comp = modeling.NewBuilder[Spec, State, Resources]().
			WithEngine(nil).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumMSHREntry:     16,
				NumSets:          64,
				NumBanks:         1,
			}).
			Build("Cache")

		m.comp.State = initialState
		next := &m.comp.State

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
		BeforeEach(func() {
			read := memprotocol.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.Address = 0x100
			read.PID = 1
			read.AccessByteSize = 64
			read.TrafficBytes = 12
			read.TrafficClass = "memprotocol.ReadReq"
			trans := transactionState{
				HasRead:            true,
				ReadMeta:           read.MsgMeta,
				ReadAddress:        read.Address,
				ReadAccessByteSize: read.AccessByteSize,
				ReadPID:            read.PID,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.DirPostPipelineBuf.Clear()
			next.DirPostPipelineBuf.PushTyped(0)
		})

		Context("mshr hit", func() {
			BeforeEach(func() {
				next := &m.comp.State
				cache.MSHRAdd(&next.MSHRState, 16, vm.PID(1), 0x100)
			})

			It("should add to MSHR", func() {

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := &m.comp.State
				Expect(next.MSHRState.Entries[0].TransactionIndices).To(HaveLen(1))
			})
		})

		Context("hit", func() {
			BeforeEach(func() {
				next := &m.comp.State
				setID := int(0x100 / uint64(64) % uint64(64))
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			})

			It("should pass transaction to bank", func() {

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := &m.comp.State
				setID := int(0x100 / uint64(64) % uint64(64))
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				Expect(block.ReadCount).To(Equal(1))
				Expect(next.Transactions[0].Action).To(Equal(bankReadHit))
			})
		})

		Context("miss, mshr miss, no need to evict", func() {
			It("should create mshr entry and fetch", func() {

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := &m.comp.State
				Expect(next.MSHRState.Entries).To(HaveLen(1))
			})
		})

		Context("miss, mshr miss, need eviction", func() {
			BeforeEach(func() {
				next := &m.comp.State
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

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := &m.comp.State
				Expect(next.Transactions[0].Action).To(Equal(bankEvictAndFetch))
			})
		})
	})

	Context("write", func() {
		BeforeEach(func() {
			write := memprotocol.WriteReq{}
			write.ID = timing.GetIDGenerator().Generate()
			write.Address = 0x100
			write.PID = 1
			write.TrafficBytes = 12
			write.TrafficClass = "memprotocol.WriteReq"
			trans := transactionState{
				HasWrite:     true,
				WriteMeta:    write.MsgMeta,
				WriteAddress: write.Address,
				WritePID:     write.PID,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.DirPostPipelineBuf.Clear()
			next.DirPostPipelineBuf.PushTyped(0)
		})

		Context("hit", func() {
			BeforeEach(func() {
				next := &m.comp.State
				setID := int(0x100 / uint64(64) % uint64(64))
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			})

			It("should send to bank", func() {

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := &m.comp.State
				Expect(next.Transactions[0].Action).To(Equal(bankWriteHit))
			})
		})

		Context("miss, write full line, no eviction", func() {
			BeforeEach(func() {
				next := &m.comp.State
				next.Transactions[0].WriteData = make([]byte, 64)
			})

			It("should send to bank", func() {

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := &m.comp.State
				Expect(next.Transactions[0].Action).To(Equal(bankWriteHit))
			})
		})
	})
})
