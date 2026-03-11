package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Flusher", func() {
	var (
		mockCtrl    *gomock.Controller
		controlPort *MockPort
		topPort     *MockPort
		bottomPort  *MockPort
		m           *middleware
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
			DirToBankBufIndices:             []bankBufState{{Indices: nil}},
			WriteBufferToBankBufIndices:     []bankBufState{{Indices: nil}},
			BankPipelineStages:              []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices:      []bankPostBufState{{Indices: nil}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &middleware{
			topPort:      topPort,
			bottomPort:   bottomPort,
			controlPort:  controlPort,
			state:        cacheStateRunning,
			evictingList: make(map[uint64]bool),
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

		m.dirStageBuffer = &stateTransBuffer{
			name:     "Cache.DirStageBuf",
			readItems:  &next.DirStageBufIndices,
			writeItems: &next.DirStageBufIndices,
			capacity: 4,
			mw:       m,
		}
		m.dirToBankBuffers = []*stateTransBuffer{{
			name:     "Cache.DirToBankBuf0",
			readItems:  &next.DirToBankBufIndices[0].Indices,
			writeItems: &next.DirToBankBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}
		m.writeBufferToBankBuffers = []*stateTransBuffer{{
			name:     "Cache.WBToBankBuf0",
			readItems:  &next.WriteBufferToBankBufIndices[0].Indices,
			writeItems: &next.WriteBufferToBankBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}
		m.mshrStageBuffer = &stateTransBuffer{
			name:     "Cache.MSHRStageBuf",
			readItems:  &next.MSHRStageBufEntries,
			writeItems: &next.MSHRStageBufEntries,
			capacity: 4,
			mw:       m,
		}
		m.writeBufferBuffer = &stateTransBuffer{
			name:     "Cache.WriteBufferBuf",
			readItems:  &next.WriteBufferBufIndices,
			writeItems: &next.WriteBufferBufIndices,
			capacity: 4,
			mw:       m,
		}
		m.dirPostBufAdapter = &stateDirPostBufAdapter{
			name:     "Cache.DirPostBuf",
			readItems:  &next.DirPostPipelineBufIndices,
			writeItems: &next.DirPostPipelineBufIndices,
			capacity: 4,
			mw:       m,
		}
		m.bankPostBufAdapters = []*stateBankPostBufAdapter{{
			name:     "Cache.BankPostBuf0",
			readItems:  &next.BankPostPipelineBufIndices[0].Indices,
			writeItems: &next.BankPostPipelineBufIndices[0].Indices,
			capacity: 4,
			mw:       m,
		}}

		m.dirStage = &directoryStage{cache: m}
		m.mshrStage = &mshrStage{cache: m}
		m.bankStages = []*bankStage{{
			cache:         m,
			bankID:        0,
			pipelineWidth: 4,
		}}
		m.writeBuffer = &writeBufferStage{
			cache:               m,
			writeBufferCapacity: 16,
			maxInflightFetch:    4,
			maxInflightEviction: 4,
		}

		f = &flusher{cache: m}
		m.flusher = f
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
			Expect(f.processingFlush).To(BeIdenticalTo(req))
			Expect(m.state).To(Equal(cacheStatePreFlushing))
		})

		It("should do nothing if there is inflight transaction", func() {
			m.state = cacheStatePreFlushing
			m.inFlightTransactions = append(
				m.inFlightTransactions, &transactionState{})
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should move to flush stage if no inflight transaction", func() {
			m.state = cacheStatePreFlushing
			m.inFlightTransactions = nil
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req

			// Set up directory with one dirty valid block
			next := m.comp.GetNextState()
			cache.DirectoryReset(&next.DirectoryState, 2, 2, 64)
			next.DirectoryState.Sets[0].Blocks[0].IsDirty = true
			next.DirectoryState.Sets[0].Blocks[0].IsValid = true

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(m.state).To(Equal(cacheStateFlushing))
			Expect(f.blockToEvict).To(HaveLen(1))
		})

		It("should send response if all the blocks are evicted", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req
			f.blockToEvict = []blockRef{}

			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(req.ID))
				})

			m.syncForTest()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.processingFlush).To(BeNil())
			Expect(m.state).To(Equal(cacheStateRunning))
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
			Expect(f.processingFlush).To(BeIdenticalTo(req))
			Expect(m.state).To(Equal(cacheStatePreFlushing))
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
			Expect(m.state).To(Equal(cacheStateRunning))
		})
	})
})
