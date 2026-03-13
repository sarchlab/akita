package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	cache2 "github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Control Stage", func() {

	var (
		mockCtrl   *gomock.Controller
		ctrlPort   *MockPort
		topPort    *MockPort
		bottomPort *MockPort
		s          *controlStage
		pmw        *pipelineMW
		co         *coalescer
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

		initialState := State{
			DirBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirBuf",
				Cap:        4,
			},
			BankBufs:      []stateutil.Buffer[int]{},
			DirPipeline:   stateutil.Pipeline[int]{Width: 4, NumStages: 2},
			DirPostBuf:    stateutil.Buffer[int]{BufferName: "Cache.DirPostBuf", Cap: 4},
			BankPipelines: []stateutil.Pipeline[int]{},
			BankPostBufs:  []stateutil.Buffer[int]{},
		}

		pmw = &pipelineMW{
			topPort:    topPort,
			bottomPort: bottomPort,
		}
		pmw.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				NumSets:          16,
				WayAssociativity: 4,
				Log2BlockSize:    6,
			}).
			Build("Cache")

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		pmw.comp.SetState(initialState)

		co = &coalescer{cache: pmw}
		pmw.coalesceStage = co

		s = &controlStage{
			ctrlPort:  ctrlPort,
			pipeline:  pmw,
			coalescer: co,
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
		next := pmw.comp.GetNextState()
		next.Transactions = append(next.Transactions, transactionState{})
		next.NumTransactions = 1

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
