package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
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
			Return(sim.RemotePort("ControlPort")).
			AnyTimes()
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		initialState := State{
			CacheState:   int(cacheStateRunning),
			EvictingList: make(map[uint64]bool),
			DirStageBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirStageBuf", Cap: 4,
			},
			DirToBankBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.DirToBankBuf", Cap: 4,
			}},
			WriteBufferToBankBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.WBToBankBuf", Cap: 4,
			}},
			MSHRStageBuf: stateutil.Buffer[int]{
				BufferName: "Cache.MSHRStageBuf", Cap: 4,
			},
			WriteBufferBuf: stateutil.Buffer[int]{
				BufferName: "Cache.WriteBufferBuf", Cap: 4,
			},
			DirPipeline: stateutil.Pipeline[int]{Width: 4, NumStages: 0},
			DirPostPipelineBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirPostBuf", Cap: 4,
			},
			BankPipelines: []stateutil.Pipeline[int]{{Width: 4, NumStages: 10}},
			BankPostPipelineBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.BankPostBuf", Cap: 4,
			}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &pipelineMW{
			topPort:    topPort,
			bottomPort: bottomPort,
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumSets:          64,
				NumBanks:         1,
			}).
			Build("Cache")

		m.comp.SetState(initialState)
		next := m.comp.GetNextState()

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
		m.syncForTest()

		ret := f.Tick()
		Expect(ret).To(BeFalse())
	})

	Context("flush without reset", func() {
		It("should start flushing", func() {
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next := m.comp.GetNextState()
			Expect(next.HasProcessingFlush).To(BeTrue())
			Expect(cacheState(next.CacheState)).To(Equal(cacheStatePreFlushing))
		})

		It("should do nothing if there is inflight transaction", func() {
			next := m.comp.GetNextState()
			next.CacheState = int(cacheStatePreFlushing)
			next.Transactions = append(
				next.Transactions, transactionState{})
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				MsgMeta: sim.MsgMeta{
					ID: sim.GetIDGenerator().Generate(),
				},
			}

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should move to flush stage if no inflight transaction", func() {
			next := m.comp.GetNextState()
			next.CacheState = int(cacheStatePreFlushing)
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				MsgMeta: sim.MsgMeta{
					ID: sim.GetIDGenerator().Generate(),
				},
			}

			// Set up directory with one dirty valid block
			cache.DirectoryReset(&next.DirectoryState, 2, 2, 64)
			next.DirectoryState.Sets[0].Blocks[0].IsDirty = true
			next.DirectoryState.Sets[0].Blocks[0].IsValid = true

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next = m.comp.GetNextState()
			Expect(cacheState(next.CacheState)).To(Equal(cacheStateFlushing))
			Expect(next.FlusherBlockToEvictRefs).To(HaveLen(1))
		})

		It("should send response if all the blocks are evicted", func() {
			next := m.comp.GetNextState()
			next.CacheState = int(cacheStateFlushing)
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				MsgMeta: sim.MsgMeta{
					ID: sim.GetIDGenerator().Generate(),
				},
			}
			next.FlusherBlockToEvictRefs = []blockRef{}

			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(next.ProcessingFlush.MsgMeta.ID))
				})

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next = m.comp.GetNextState()
			Expect(next.HasProcessingFlush).To(BeFalse())
			Expect(cacheState(next.CacheState)).To(Equal(cacheStateRunning))
		})
	})

	Context("flush with reset", func() {
		It("should remove inflight state", func() {
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.DiscardInflight = true
			req.TrafficClass = "cache.FlushReq"

			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next := m.comp.GetNextState()
			Expect(next.HasProcessingFlush).To(BeTrue())
			Expect(cacheState(next.CacheState)).To(Equal(cacheStatePreFlushing))
		})
	})

	Context("restarting", func() {
		It("should stall if cannot send to control port", func() {
			req := &cache.RestartReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.RestartReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().CanSend().Return(false)

			m.syncForTest()

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should restart", func() {
			req := &cache.RestartReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.RestartReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any())
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			bottomPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			m.syncForTest()

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeTrue())
			next := m.comp.GetNextState()
			Expect(cacheState(next.CacheState)).To(Equal(cacheStateRunning))
		})
	})
})
