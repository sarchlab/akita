package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
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

		initialState := state{
			CacheState:   int(cacheStateRunning),
			EvictingList: make(map[uint64]bool),
			DirStageBuf:  queueing.MakeBuffer[int](4),
			DirToBankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			WriteBufferToBankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			MSHRStageBuf:       queueing.MakeBuffer[int](4),
			WriteBufferBuf:     queueing.MakeBuffer[int](4),
			DirPipeline:        queueing.MakePipeline[int](4, 0),
			DirPostPipelineBuf: queueing.MakeBuffer[int](4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.MakePipeline[int](4, 10),
			},
			BankPostPipelineBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		spec := stageTestSpec()
		spec.NumMSHREntry = 16
		comp := buildStageTestComp(spec,
			Resources{Storage: mem.NewStorage(spec.TotalByteSize)},
			makePorts("Cache", 4))

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		ds = m.dirStage
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("read", func() {
		BeforeEach(func() {
			read := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x100,
				PID:            1,
				AccessByteSize: 64},
				ID: m.comp.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			trans := transactionState{
				HasRead:            true,
				ReadMeta:           read,
				ReadAddress:        read.Payload.(memprotocol.ReadReq).Address,
				ReadAccessByteSize: read.Payload.(memprotocol.ReadReq).AccessByteSize,
				ReadPID:            read.Payload.(memprotocol.ReadReq).PID,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.DirPostPipelineBuf.Clear()
			next.DirPostPipelineBuf.Push(0)
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
				setID := cache.DirectorySetID(0x100, 64, 64)
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			})

			It("should pass transaction to bank", func() {

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				next := &m.comp.State
				setID := cache.DirectorySetID(0x100, 64, 64)
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
				setID := cache.DirectorySetID(0x100, 64, 64)
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
			write := messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x100,
				PID:     1},
				ID: m.comp.NewID(),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.WriteReq"}

			trans := transactionState{
				HasWrite:     true,
				WriteMeta:    write,
				WriteAddress: write.Payload.(memprotocol.WriteReq).Address,
				WritePID:     write.Payload.(memprotocol.WriteReq).PID,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.DirPostPipelineBuf.Clear()
			next.DirPostPipelineBuf.Push(0)
		})

		Context("hit", func() {
			BeforeEach(func() {
				next := &m.comp.State
				setID := cache.DirectorySetID(0x100, 64, 64)
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
