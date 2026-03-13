package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("TopParser", func() {
	var (
		mockCtrl *gomock.Controller
		m        *pipelineMW
		parser   *topParser
		port     *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		port = NewMockPort(mockCtrl)

		initialState := State{
			CacheState:   int(cacheStateRunning),
			EvictingList: make(map[uint64]bool),
			DirStageBuf: queueing.Buffer[int]{
				BufferName: "Cache.DirStageBuf", Cap: 4,
			},
			DirToBankBufs: []queueing.Buffer[int]{{
				BufferName: "Cache.DirToBankBuf", Cap: 4,
			}},
			WriteBufferToBankBufs: []queueing.Buffer[int]{{
				BufferName: "Cache.WBToBankBuf", Cap: 4,
			}},
			MSHRStageBuf: queueing.Buffer[int]{
				BufferName: "Cache.MSHRStageBuf", Cap: 4,
			},
			WriteBufferBuf: queueing.Buffer[int]{
				BufferName: "Cache.WriteBufferBuf", Cap: 4,
			},
			DirPipeline: queueing.Pipeline[int]{Width: 4, NumStages: 0},
			DirPostPipelineBuf: queueing.Buffer[int]{
				BufferName: "Cache.DirPostBuf", Cap: 4,
			},
			BankPipelines: []queueing.Pipeline[int]{{Width: 4, NumStages: 10}},
			BankPostPipelineBufs: []queueing.Buffer[int]{{
				BufferName: "Cache.BankPostBuf", Cap: 4,
			}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &pipelineMW{
			topPort: port,
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				NumReqPerCycle: 4,
				Log2BlockSize:  6,
			}).
			Build("Cache")

		m.comp.SetState(initialState)

		parser = &topParser{
			cache: m,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should return if no req to parse", func() {
		port.EXPECT().PeekIncoming().Return(nil)
		m.syncForTest()

		ret := parser.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should return if the cache is not in running stage", func() {
		next := m.comp.GetNextState()
		next.CacheState = int(cacheStateFlushing)
		m.syncForTest()

		ret := parser.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should parse read from top", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x100
		read.AccessByteSize = 64
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		port.EXPECT().PeekIncoming().Return(read)
		port.EXPECT().RetrieveIncoming().Return(read)

		m.syncForTest()

		parser.Tick()

		next := m.comp.GetNextState()
		Expect(next.Transactions).To(HaveLen(1))
		Expect(next.Transactions[0].HasRead).To(BeTrue())
		Expect(next.Transactions[0].ReadAddress).To(Equal(uint64(0x100)))
		Expect(next.Transactions[0].ReadAccessByteSize).To(Equal(uint64(64)))
	})

	It("should parse write from top", func() {
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Address = 0x100
		write.TrafficBytes = 12
		write.TrafficClass = "mem.WriteReq"

		port.EXPECT().PeekIncoming().Return(write)
		port.EXPECT().RetrieveIncoming().Return(write)

		m.syncForTest()

		parser.Tick()

		next := m.comp.GetNextState()
		Expect(next.Transactions).To(HaveLen(1))
		Expect(next.Transactions[0].HasWrite).To(BeTrue())
		Expect(next.Transactions[0].WriteAddress).To(Equal(uint64(0x100)))
	})
})
