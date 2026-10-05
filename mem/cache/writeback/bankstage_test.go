package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/queueing"
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
		dummy := messaging.Msg{Payload: memprotocol.DataReadyRsp{},
			Src:          topPort.AsRemote(),
			Dst:          messaging.RemotePort("SomeSrc"),
			TrafficClass: "rsp"}

		Expect(topPort.CanSend()).To(BeTrue())
		topPort.Send(dummy)
	}

	BeforeEach(func() {
		storage = mem.NewStorage(4 * mem.KB)

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
		spec.BankLatency = 10

		// The stage sends on the Top port, so the test gives it a single-slot
		// Top port to simulate a busy port.
		ports := makePorts("Cache", 4)
		ports.Top = messaging.NewPort("Cache.Top", 1, 1)
		comp := buildStageTestComp(spec, Resources{Storage: storage}, ports)
		topPort = comp.Ports.Top

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		bs = m.bankStages[0]
	})

	It("completes a later transaction while preserving a blocked zero-index head", func() {
		next := &m.comp.State
		fillTop()
		next.Transactions = []transactionState{
			{Action: bankReadHit},
			{Action: bankEvict},
			{Action: bankWriteHit},
		}
		for _, idx := range []int{0, 1, 2} {
			next.BankPostPipelineBufs[0].Push(idx)
		}
		next.BankInflightTransCounts[0] = 3
		next.BankDownwardInflightTransCounts[0] = 1

		Expect(bs.finalizeTrans()).To(BeTrue())
		Expect(next.BankPostPipelineBufs[0].Elements()).To(Equal([]int{0, 2}))
		Expect(next.WriteBufferBuf.Elements()).To(Equal([]int{1}))
		Expect(next.Transactions[0].Removed).To(BeFalse())
		Expect(next.Transactions[1].Action).To(Equal(writeBufferFlush))
		Expect(next.BankInflightTransCounts[0]).To(Equal(2))
		Expect(next.BankDownwardInflightTransCounts[0]).To(Equal(0))
		Expect(bs.finalizeTrans()).To(BeFalse())
		Expect(next.BankPostPipelineBufs[0].Elements()).To(Equal([]int{0, 2}))
	})

	Context("completing a read hit transaction", func() {
		BeforeEach(func() {
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1

			storage.Write(0x40, []byte{1, 2, 3, 4, 5, 6, 7, 8})

			read := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x104,
				AccessByteSize: 4},
				ID:  m.comp.NewID(),
				Src: messaging.RemotePort("Agent"),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			trans := transactionState{
				HasRead:            true,
				ReadMeta:           read,
				ReadAddress:        read.Payload.(memprotocol.ReadReq).Address,
				ReadAccessByteSize: read.Payload.(memprotocol.ReadReq).AccessByteSize,
				ReadPID:            read.Payload.(memprotocol.ReadReq).PID,
				BlockSetID:         0,
				BlockWayID:         0,
				HasBlock:           true,
				Action:             bankReadHit,
			}
			next.Transactions = []transactionState{trans}

			// Put transaction in bank post-pipeline buffer
			next.BankPostPipelineBufs[0].Push(0)
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

			out, _ := topPort.RetrieveOutgoing()
			dr := out
			Expect(dr.Payload.(memprotocol.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))
		})
	})

	Context("completing a write-hit transaction", func() {
		BeforeEach(func() {
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1
			block.IsLocked = true

			write := messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x104,
				Data:    []byte{5, 6, 7, 8}},
				ID:  m.comp.NewID(),
				Src: messaging.RemotePort("Agent"),

				TrafficBytes: len([]byte{5, 6, 7, 8}) + 12,
				TrafficClass: "memprotocol.WriteReq"}

			trans := transactionState{
				HasWrite:     true,
				WriteMeta:    write,
				WriteAddress: write.Payload.(memprotocol.WriteReq).Address,
				WriteData:    write.Payload.(memprotocol.WriteReq).Data,
				WritePID:     write.Payload.(memprotocol.WriteReq).PID,
				BlockSetID:   0,
				BlockWayID:   0,
				HasBlock:     true,
				Action:       bankWriteHit,
			}
			next.Transactions = []transactionState{trans}
			next.BankPostPipelineBufs[0].Push(0)
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
			data := storage.Read(0x44, 4)
			Expect(data).To(Equal([]byte{5, 6, 7, 8}))
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsDirty).To(BeTrue())
			Expect(next.Transactions[0].Removed).To(BeTrue())
			Expect(next.BankInflightTransCounts[0]).To(Equal(0))

			out, _ := topPort.RetrieveOutgoing()
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
			next.BankPostPipelineBufs[0].Push(0)
			next.BankInflightTransCounts[0] = 1
		})

		It("should write to storage and send to mshr stage", func() {

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			next := &m.comp.State
			writtenData := storage.Read(0x40, 64)
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
			next.BankPostPipelineBufs[0].Push(0)
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
