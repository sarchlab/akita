package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Flusher", func() {
	var (
		mockCtrl    *gomock.Controller
		controlPort *MockPort
		topPort     *MockPort
		bottomPort  *MockPort
		m           *pipelineMW
		f           *flusher
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("ControlPort")).
			AnyTimes()
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("TopPort")).
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("BottomPort")).
			AnyTimes()

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
			topPort:    topPort,
			bottomPort: bottomPort,
		}
		m.comp = modeling.NewBuilder[Spec, State, modeling.None]().
			WithEngine(nil).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumSets:          64,
				NumBanks:         1,
			}).
			Build("Cache")

		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		m.dirStage = &directoryStage{cache: m}
		m.mshrStage = &mshrStage{cache: m}
		m.bankStages = []*bankStage{{
			cache:         m,
			bankID:        0,
			pipelineWidth: 4,
		}}
		m.writeBuffer = &writeBufferStage{
			cache: m,
		}

		f = &flusher{pipeline: m, ctrlPort: controlPort}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		controlPort.EXPECT().PeekIncoming().Return(nil)

		ret := f.Tick()
		Expect(ret).To(BeFalse())
	})

	Context("flush without reset", func() {
		It("should start flushing", func() {
			req := &mem.ControlReq{Command: mem.CmdFlush}
			req.ID = timing.GetIDGenerator().Generate()
			req.TrafficClass = "mem.ControlReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next := &m.comp.State
			Expect(next.HasProcessingFlush).To(BeTrue())
			Expect(cacheState(next.CacheState)).To(Equal(cacheStatePreFlushing))
		})

		It("should do nothing if there is inflight transaction", func() {
			next := &m.comp.State
			next.CacheState = int(cacheStatePreFlushing)
			next.Transactions = append(
				next.Transactions, transactionState{})
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				MsgMeta: messaging.MsgMeta{
					ID: timing.GetIDGenerator().Generate(),
				},
			}

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should move to flush stage if no inflight transaction", func() {
			next := &m.comp.State
			next.CacheState = int(cacheStatePreFlushing)
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				MsgMeta: messaging.MsgMeta{
					ID: timing.GetIDGenerator().Generate(),
				},
			}

			// Set up directory with one dirty valid block
			cache.DirectoryReset(&next.DirectoryState, 2, 2, 64)
			next.DirectoryState.Sets[0].Blocks[0].IsDirty = true
			next.DirectoryState.Sets[0].Blocks[0].IsValid = true

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(cacheState(next.CacheState)).To(Equal(cacheStateFlushing))
			Expect(next.FlusherBlockToEvictRefs).To(HaveLen(1))
		})

		It("should send response if all the blocks are evicted", func() {
			next := &m.comp.State
			next.CacheState = int(cacheStateFlushing)
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				MsgMeta: messaging.MsgMeta{
					ID: timing.GetIDGenerator().Generate(),
				},
			}
			next.FlusherBlockToEvictRefs = []blockRef{}

			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any()).
				Do(func(msg messaging.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(next.ProcessingFlush.MsgMeta.ID))
				})

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(next.HasProcessingFlush).To(BeFalse())
			Expect(cacheState(next.CacheState)).To(Equal(cacheStateRunning))
		})
	})

	Context("flush with reset", func() {
		It("should remove inflight state", func() {
			req := &mem.ControlReq{Command: mem.CmdFlush}
			req.ID = timing.GetIDGenerator().Generate()
			req.DiscardInflight = true
			req.TrafficClass = "mem.ControlReq"

			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next := &m.comp.State
			Expect(next.HasProcessingFlush).To(BeTrue())
			Expect(cacheState(next.CacheState)).To(Equal(cacheStatePreFlushing))
		})
	})

	Context("restarting", func() {
		It("should stall if cannot send to control port", func() {
			req := &mem.ControlReq{Command: mem.CmdEnable}
			req.ID = timing.GetIDGenerator().Generate()
			req.TrafficClass = "mem.ControlReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().CanSend().Return(false)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should restart", func() {
			req := &mem.ControlReq{Command: mem.CmdEnable}
			req.ID = timing.GetIDGenerator().Generate()
			req.TrafficClass = "mem.ControlReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any())
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			bottomPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeTrue())
			next := &m.comp.State
			Expect(cacheState(next.CacheState)).To(Equal(cacheStateRunning))
		})
	})
})
