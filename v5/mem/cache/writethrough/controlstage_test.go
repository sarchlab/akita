package writethrough

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	cache2 "github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Control Stage", func() {

	var (
		mockCtrl     *gomock.Controller
		ctrlPort     *MockPort
		topPort      *MockPort
		bottomPort   *MockPort
		transactions []*transactionState
		s            *controlStage
		mw           *middleware
		co           *coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		ctrlPort = NewMockPort(mockCtrl)
		ctrlPort.EXPECT().
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

		transactions = nil

		initialState := State{
			BankBufIndices:             []bankBufState{},
			BankPipelineStages:         []bankPipelineState{},
			BankPostPipelineBufIndices: []bankPostBufState{},
		}

		mw = &middleware{
			topPort:    topPort,
			bottomPort: bottomPort,
		}
		mw.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				NumSets:          16,
				WayAssociativity: 4,
				Log2BlockSize:    6,
			}).
			Build("Cache")

		mw.comp.SetState(initialState)

		next := mw.comp.GetNextState()

		// Initialize directoryState
		cache.DirectoryReset(&next.DirectoryState, 16, 4, 64)

		// Create dir buf adapter
		mw.dirBufAdapter = &stateTransBuffer{
			name:     "Cache.DirBuf",
			items:    &next.DirBufIndices,
			capacity: 4,
			mw:       mw,
		}
		mw.bankBufAdapters = nil

		co = &coalescer{cache: mw}
		mw.coalesceStage = co

		s = &controlStage{
			ctrlPort:     ctrlPort,
			transactions: &transactions,
			cache:        mw,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		ctrlPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should wait for the cache to finish transactions", func() {
		transactions = []*transactionState{{}}
		s.cache.transactions = transactions
		flushReq := &cache2.FlushReq{}
		flushReq.ID = sim.GetIDGenerator().Generate()
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "ctrl"
		flushReq.DiscardInflight = false
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().PeekIncoming().Return(flushReq)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should reset directory", func() {
		flushReq := &cache2.FlushReq{}
		flushReq.ID = sim.GetIDGenerator().Generate()
		flushReq.InvalidateAllCachelines = true
		flushReq.DiscardInflight = true
		flushReq.PauseAfterFlushing = true
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "ctrl"
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().Send(gomock.Any()).Do(func(msg sim.Msg) {
			Expect(msg.Meta().RspTo).To(Equal(flushReq.ID))
		})

		topPort.EXPECT().PeekIncoming().Return(nil)
		bottomPort.EXPECT().PeekIncoming().Return(nil)

		ctrlPort.EXPECT().PeekIncoming().Return(flushReq)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(s.currFlushReq).To(BeNil())
	})

})
