package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"

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
			DirBuf: queueing.Buffer[int]{
				BufferName: "Cache.DirBuf",
				Cap:        4,
			},
			BankBufs:      []queueing.Buffer[int]{},
			DirPipeline:   queueing.Pipeline[int]{Width: 4, NumStages: 2},
			DirPostBuf:    queueing.Buffer[int]{BufferName: "Cache.DirPostBuf", Cap: 4},
			BankPipelines: []queueing.Pipeline[int]{},
			BankPostBufs:  []queueing.Buffer[int]{},
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

		s = &controlStage{
			ctrlPort: ctrlPort,
			pipeline: pmw,
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

		flushReq := &mem.ControlReq{Command: mem.CmdFlush}
		flushReq.ID = sim.GetIDGenerator().Generate()
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "mem.ControlReq"
		flushReq.DiscardInflight = false

		// Store flush request in State instead of controlStage field
		next.HasProcessingFlush = true
		next.ProcessingFlush = flushReqState{
			MsgMeta:         flushReq.MsgMeta,
			DiscardInflight: flushReq.DiscardInflight,
			PauseAfter:      flushReq.PauseAfter,
		}

		ctrlPort.EXPECT().PeekIncoming().Return(flushReq)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should reset directory", func() {
		flushReq := &mem.ControlReq{Command: mem.CmdFlush}
		flushReq.ID = sim.GetIDGenerator().Generate()
		flushReq.InvalidateAfter = true
		flushReq.DiscardInflight = true
		flushReq.PauseAfter = true
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "mem.ControlReq"

		// Store flush request in State instead of controlStage field
		next := pmw.comp.GetNextState()
		next.HasProcessingFlush = true
		next.ProcessingFlush = flushReqState{
			MsgMeta:         flushReq.MsgMeta,
			DiscardInflight: flushReq.DiscardInflight,
			PauseAfter:      flushReq.PauseAfter,
		}

		ctrlPort.EXPECT().Send(gomock.Any()).Do(func(msg sim.Msg) {
			Expect(msg.Meta().RspTo).To(Equal(flushReq.ID))
		})

		topPort.EXPECT().PeekIncoming().Return(nil)
		bottomPort.EXPECT().PeekIncoming().Return(nil)

		ctrlPort.EXPECT().PeekIncoming().Return(flushReq)

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeTrue())
		next = pmw.comp.GetNextState()
		Expect(next.HasProcessingFlush).To(BeFalse())
	})

})
