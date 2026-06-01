package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
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
			topPort: port,
		}
		m.comp = modeling.NewBuilder[Spec, State, modeling.None]().
			WithEngine(nil).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				NumReqPerCycle: 4,
				Log2BlockSize:  6,
			}).
			Build("Cache")

		m.comp.State = initialState

		parser = &topParser{
			cache: m,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should return if no req to parse", func() {
		port.EXPECT().PeekIncoming().Return(nil)

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
		read := &mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Address = 0x100
		read.AccessByteSize = 64
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		port.EXPECT().PeekIncoming().Return(read)
		port.EXPECT().RetrieveIncoming().Return(read)

		parser.Tick()

		next := &m.comp.State
		Expect(next.Transactions).To(HaveLen(1))
		Expect(next.Transactions[0].HasRead).To(BeTrue())
		Expect(next.Transactions[0].ReadAddress).To(Equal(uint64(0x100)))
		Expect(next.Transactions[0].ReadAccessByteSize).To(Equal(uint64(64)))
	})

	It("should parse write from top", func() {
		write := &mem.WriteReq{}
		write.ID = timing.GetIDGenerator().Generate()
		write.Address = 0x100
		write.TrafficBytes = 12
		write.TrafficClass = "mem.WriteReq"

		port.EXPECT().PeekIncoming().Return(write)
		port.EXPECT().RetrieveIncoming().Return(write)

		parser.Tick()

		next := &m.comp.State
		Expect(next.Transactions).To(HaveLen(1))
		Expect(next.Transactions[0].HasWrite).To(BeTrue())
		Expect(next.Transactions[0].WriteAddress).To(Equal(uint64(0x100)))
	})
})
