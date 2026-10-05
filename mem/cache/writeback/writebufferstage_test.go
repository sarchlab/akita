package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
)

var _ = Describe("WriteBufferStage", func() {
	var (
		m          *pipelineMW
		wb         *writeBufferStage
		bottomPort messaging.Port
	)

	BeforeEach(func() {
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
		spec.WriteBufferCapacity = 16
		spec.MaxInflightFetch = 4
		spec.MaxInflightEviction = 4
		comp := buildStageTestComp(spec,
			Resources{
				Storage:             mem.NewStorage(spec.TotalByteSize),
				AddressToPortMapper: &mem.SinglePortMapper{Port: "DRAM"},
			},
			makePorts("Cache", 4))
		bottomPort = comp.Ports.Bottom

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState

		wb = m.writeBuffer
	})

	It("should do nothing if no transactions", func() {
		ret := wb.Tick()

		Expect(ret).To(BeFalse())
	})

	Context("processing new writeBufferFetch transactions", func() {
		It("should fetch from bottom", func() {
			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID:           m.comp.NewID(),
				TrafficClass: "memprotocol.ReadReq"}

			trans := transactionState{
				Action:       writeBufferFetch,
				FetchAddress: 0x100,
				FetchPID:     1,
				BlockSetID:   0,
				BlockWayID:   0,
				HasBlock:     true,
				HasRead:      true,
				ReadMeta:     read,
				ReadAddress:  0x100,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.WriteBufferBuf.Clear()
			next.WriteBufferBuf.Push(0)

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(next.InflightFetchIndices).To(HaveLen(1))

			out, _ := bottomPort.RetrieveOutgoing()
			Expect(out).NotTo(BeNil())
		})

		It("should stall fetch if too many inflight fetches", func() {
			next := &m.comp.State
			next.InflightFetchIndices = []int{10, 11, 12, 13}

			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID:           m.comp.NewID(),
				TrafficClass: "memprotocol.ReadReq"}

			trans := transactionState{
				Action:       writeBufferFetch,
				FetchAddress: 0x100,
				HasRead:      true,
				ReadMeta:     read,
				ReadAddress:  0x100,
			}
			next.Transactions = []transactionState{trans}
			next.WriteBufferBuf.Clear()
			next.WriteBufferBuf.Push(0)

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
			_, present0 := bottomPort.PeekOutgoing()
			Expect(present0).To(BeFalse())
		})
	})

	Context("writing evictions", func() {
		It("should send eviction to bottom", func() {
			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID:           m.comp.NewID(),
				TrafficClass: "memprotocol.ReadReq"}

			trans := transactionState{
				EvictingAddr: 0x200,
				EvictingPID:  2,
				EvictingData: make([]byte, 64),
				HasRead:      true,
				ReadMeta:     read,
				ReadAddress:  0x200,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.PendingEvictionIndices = []int{0}

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(next.PendingEvictionIndices).To(HaveLen(0))
			Expect(next.InflightEvictionIndices).To(HaveLen(1))

			out, _ := bottomPort.RetrieveOutgoing()
			Expect(out).NotTo(BeNil())
		})

		It("should stall if too many inflight evictions", func() {
			next := &m.comp.State
			next.InflightEvictionIndices = []int{10, 11, 12, 13}
			next.PendingEvictionIndices = []int{0}
			next.Transactions = []transactionState{{}}

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
			_, present1 := bottomPort.PeekOutgoing()
			Expect(present1).To(BeFalse())
		})
	})

	Context("processing responses", func() {
		It("should process write done response", func() {
			evictWrite := messaging.Msg{Payload: memprotocol.WriteReq{},
				ID:           9001,
				TrafficClass: "memprotocol.WriteReq"}

			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID:           m.comp.NewID(),
				TrafficClass: "memprotocol.ReadReq"}

			trans := transactionState{
				HasEvictionWriteReq:  true,
				EvictionWriteReqMeta: evictWrite,
				HasRead:              true,
				ReadMeta:             read,
				ReadAddress:          0,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.InflightEvictionIndices = []int{0}

			rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				RspTo:        9001,
				TrafficClass: "memprotocol.WriteDoneRsp"}

			bottomPort.Deliver(rsp)

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(next.InflightEvictionIndices).To(HaveLen(0))
			_, present2 := bottomPort.PeekIncoming()
			Expect(present2).To(BeFalse())
		})
	})
})
