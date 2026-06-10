package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("Bank Stage", func() {
	var (
		m       *pipelineMW
		bs      *bankStage
		storage *mem.Storage
		topPort messaging.Port
	)

	// fillTop pre-fills topPort's single outgoing slot so the next CanSend
	// returns false, simulating a busy port.
	fillTop := func() {
		dummy := memprotocol.DataReadyRsp{}
		dummy.Src = topPort.AsRemote()
		dummy.Dst = messaging.RemotePort("SomeSrc")
		dummy.TrafficClass = "rsp"
		Expect(topPort.CanSend()).To(BeTrue())
		topPort.Send(dummy)
	}

	BeforeEach(func() {
		storage = mem.NewStorage(4 * mem.KB)

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

		m = &pipelineMW{
			storage: storage,
		}
		m.comp = modeling.NewBuilder[Spec, State, Resources]().
			WithEngine(timing.NewSerialEngine()).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				BankLatency:      10,
				Log2BlockSize:    6,
				WayAssociativity: 4,
				NumSets:          64,
				NumBanks:         1,
				NumReqPerCycle:   4,
			}).
			Build("Cache")

		// The stage resolves the "Top" port by name, so the test assigns a real
		// single-slot port (owned by the component) and plugs a noop connection.
		topPort = messaging.NewPort(m.comp, 1, 1, "Cache.Top")
		(&ccNoopConn{}).PlugIn(topPort)
		m.comp.DeclarePort("Top")
		m.comp.AssignPort("Top", topPort)

		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		bs = &bankStage{
			cache:         m,
			bankID:        0,
			pipelineWidth: 4,
		}
		m.bankStages = []*bankStage{bs}
	})

	Context("completing a read hit transaction", func() {
		BeforeEach(func() {
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1

			storage.Write(0x40, []byte{1, 2, 3, 4, 5, 6, 7, 8})

			read := memprotocol.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.Src = messaging.RemotePort("Agent")
			read.Address = 0x104
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "memprotocol.ReadReq"
			trans := transactionState{
				HasRead:            true,
				ReadMeta:           read.MsgMeta,
				ReadAddress:        read.Address,
				ReadAccessByteSize: read.AccessByteSize,
				ReadPID:            read.PID,
				BlockSetID:         0,
				BlockWayID:         0,
				HasBlock:           true,
				Action:             bankReadHit,
			}
			next.Transactions = []transactionState{trans}

			// Put transaction in bank post-pipeline buffer
			next.BankPostPipelineBufs[0].PushTyped(0)
			next.BankInflightTransCounts[0] = 1
		})

		It("should stall if send buffer is full", func() {
			fillTop()

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			next := &m.comp.State
			Expect(next.BankInflightTransCounts[0]).To(Equal(1))
		})

		It("should read and send response", func() {
			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			Expect(block.ReadCount).To(Equal(0))
			Expect(next.Transactions[0].Removed).To(BeTrue())
			Expect(next.BankInflightTransCounts[0]).To(Equal(0))

			out := topPort.RetrieveOutgoing()
			dr := out.(memprotocol.DataReadyRsp)
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
		})
	})

	Context("completing a write-hit transaction", func() {
		BeforeEach(func() {
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1
			block.IsLocked = true

			write := memprotocol.WriteReq{}
			write.ID = timing.GetIDGenerator().Generate()
			write.Src = messaging.RemotePort("Agent")
			write.Address = 0x104
			write.Data = []byte{5, 6, 7, 8}
			write.TrafficBytes = len([]byte{5, 6, 7, 8}) + 12
			write.TrafficClass = "memprotocol.WriteReq"
			trans := transactionState{
				HasWrite:     true,
				WriteMeta:    write.MsgMeta,
				WriteAddress: write.Address,
				WriteData:    write.Data,
				WritePID:     write.PID,
				BlockSetID:   0,
				BlockWayID:   0,
				HasBlock:     true,
				Action:       bankWriteHit,
			}
			next.Transactions = []transactionState{trans}
			next.BankPostPipelineBufs[0].PushTyped(0)
			next.BankInflightTransCounts[0] = 1
		})

		It("should stall if send buffer is full", func() {
			fillTop()

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			next := &m.comp.State
			Expect(next.BankInflightTransCounts[0]).To(Equal(1))
		})

		It("should write and send response", func() {
			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			data, _ := storage.Read(0x44, 4)
			Expect(data).To(Equal([]byte{5, 6, 7, 8}))
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsDirty).To(BeTrue())
			Expect(next.Transactions[0].Removed).To(BeTrue())
			Expect(next.BankInflightTransCounts[0]).To(Equal(0))

			out := topPort.RetrieveOutgoing()
			Expect(out).NotTo(BeNil())
		})
	})

	Context("completing a write fetched transaction", func() {
		BeforeEach(func() {
			next := &m.comp.State
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

			trans := transactionState{
				BlockSetID:             0,
				BlockWayID:             0,
				HasBlock:               true,
				MSHRData:               fetchedData,
				MSHRTransactionIndices: []int{},
				Action:                 bankWriteFetched,
			}
			next.Transactions = []transactionState{trans}
			next.BankPostPipelineBufs[0].PushTyped(0)
			next.BankInflightTransCounts[0] = 1
		})

		It("should write to storage and send to mshr stage", func() {

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			next := &m.comp.State
			writtenData, _ := storage.Read(0x40, 64)
			Expect(writtenData).To(Equal(next.Transactions[0].MSHRData))
			block := &next.DirectoryState.Sets[0].Blocks[0]
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsValid).To(BeTrue())
			Expect(next.BankInflightTransCounts[0]).To(Equal(0))
		})
	})

	Context("finalizing a read for eviction action", func() {
		BeforeEach(func() {
			next := &m.comp.State
			trans := transactionState{
				HasVictim:          true,
				VictimTag:          0x200,
				VictimCacheAddress: 0x300,
				VictimDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
				Action:       bankEvictAndFetch,
				EvictingAddr: 0x200,
			}
			next.Transactions = []transactionState{trans}
			next.BankPostPipelineBufs[0].PushTyped(0)
			next.BankInflightTransCounts[0] = 1
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

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			next := &m.comp.State
			Expect(next.Transactions[0].Action).To(Equal(writeBufferEvictAndFetch))
			Expect(next.Transactions[0].EvictingData).To(Equal(data))
			Expect(next.BankInflightTransCounts[0]).To(Equal(0))
		})
	})
})
