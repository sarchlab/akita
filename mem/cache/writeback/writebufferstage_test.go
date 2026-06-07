package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("WriteBufferStage", func() {
	var (
		m          *pipelineMW
		wb         *writeBufferStage
		bottomPort messaging.Port
	)

	BeforeEach(func() {
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
			WithEngine(timing.NewSerialEngine()).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				Log2BlockSize:       6,
				NumReqPerCycle:      4,
				WayAssociativity:    4,
				NumSets:             64,
				NumBanks:            1,
				AddressMapperType:   "single",
				RemotePortNames:     []string{"DRAM"},
				WriteBufferCapacity: 16,
				MaxInflightFetch:    4,
				MaxInflightEviction: 4,
			}).
			Build("Cache")

		// The stage resolves the "Bottom" port by name, so the test assigns a
		// real port (owned by the component) and plugs a noop connection.
		bottomPort = messaging.NewPort(m.comp, 4, 4, "Cache.Bottom")
		(&ccNoopConn{}).PlugIn(bottomPort)
		m.comp.DeclarePort("Bottom")
		m.comp.AssignPort("Bottom", bottomPort)

		m.comp.State = initialState

		wb = &writeBufferStage{
			cache: m,
		}
		m.writeBuffer = wb
	})

	It("should do nothing if no transactions", func() {
		ret := wb.Tick()

		Expect(ret).To(BeFalse())
	})

	Context("processing new writeBufferFetch transactions", func() {
		It("should fetch from bottom", func() {
			read := mem.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := transactionState{
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

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.WriteBufferBuf.Clear()
			next.WriteBufferBuf.PushTyped(0)

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(next.InflightFetchIndices).To(HaveLen(1))

			out := bottomPort.RetrieveOutgoing()
			Expect(out).NotTo(BeNil())
		})

		It("should stall fetch if too many inflight fetches", func() {
			next := &m.comp.State
			next.InflightFetchIndices = []int{10, 11, 12, 13}

			read := mem.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := transactionState{
				Action:       writeBufferFetch,
				FetchAddress: 0x100,
				HasRead:      true,
				ReadMeta:     read.MsgMeta,
				ReadAddress:  0x100,
			}
			next.Transactions = []transactionState{trans}
			next.WriteBufferBuf.Clear()
			next.WriteBufferBuf.PushTyped(0)

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
			Expect(bottomPort.PeekOutgoing()).To(BeNil())
		})
	})

	Context("writing evictions", func() {
		It("should send eviction to bottom", func() {
			read := mem.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := transactionState{
				EvictingAddr: 0x200,
				EvictingPID:  2,
				EvictingData: make([]byte, 64),
				HasRead:      true,
				ReadMeta:     read.MsgMeta,
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

			out := bottomPort.RetrieveOutgoing()
			Expect(out).NotTo(BeNil())
		})

		It("should stall if too many inflight evictions", func() {
			next := &m.comp.State
			next.InflightEvictionIndices = []int{10, 11, 12, 13}
			next.PendingEvictionIndices = []int{0}
			next.Transactions = []transactionState{{}}

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
			Expect(bottomPort.PeekOutgoing()).To(BeNil())
		})
	})

	Context("processing responses", func() {
		It("should process write done response", func() {
			evictWrite := mem.WriteReq{}
			evictWrite.ID = 9001
			evictWrite.TrafficClass = "mem.WriteReq"

			read := mem.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.TrafficClass = "mem.ReadReq"
			trans := transactionState{
				HasEvictionWriteReq:  true,
				EvictionWriteReqMeta: evictWrite.MsgMeta,
				HasRead:              true,
				ReadMeta:             read.MsgMeta,
				ReadAddress:          0,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.InflightEvictionIndices = []int{0}

			rsp := mem.WriteDoneRsp{}
			rsp.RspTo = 9001
			rsp.TrafficClass = "mem.WriteDoneRsp"

			bottomPort.Deliver(rsp)

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(next.InflightEvictionIndices).To(HaveLen(0))
			Expect(bottomPort.PeekIncoming()).To(BeNil())
		})
	})
})
