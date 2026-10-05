package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/queueing"
)

var _ = Describe("TopParser", func() {
	var (
		m       *pipelineMW
		parser  *topParser
		topPort messaging.Port
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
		comp := buildStageTestComp(spec,
			Resources{Storage: mem.NewStorage(spec.TotalByteSize)},
			makePorts("Cache", 4))
		topPort = comp.Ports.Top

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState

		parser = m.topParser
	})

	It("should return if no req to parse", func() {
		ret := parser.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should return if the cache is not in running stage", func() {
		next := &m.comp.State
		next.CacheState = int(cacheStateFlushing)

		ret := parser.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should parse read from top", func() {
		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 64},
			ID: m.comp.NewID(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		topPort.Deliver(read)

		parser.Tick()

		next := &m.comp.State
		Expect(next.Transactions).To(HaveLen(1))
		Expect(next.Transactions[0].HasRead).To(BeTrue())
		Expect(next.Transactions[0].ReadAddress).To(Equal(uint64(0x100)))
		Expect(next.Transactions[0].ReadAccessByteSize).To(Equal(uint64(64)))
		_, present0 := topPort.PeekIncoming()
		Expect(present0).To(BeFalse())
	})

	It("should parse write from top", func() {
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100},
			ID: m.comp.NewID(),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.WriteReq"}

		topPort.Deliver(write)

		parser.Tick()

		next := &m.comp.State
		Expect(next.Transactions).To(HaveLen(1))
		Expect(next.Transactions[0].HasWrite).To(BeTrue())
		Expect(next.Transactions[0].WriteAddress).To(Equal(uint64(0x100)))
		_, present1 := topPort.PeekIncoming()
		Expect(present1).To(BeFalse())
	})
})
