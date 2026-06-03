package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("Control Stage", func() {

	var (
		ctrlPort   messaging.Port
		topPort    messaging.Port
		bottomPort messaging.Port
		s          *controlStage
		pmw        *pipelineMW
	)

	BeforeEach(func() {
		conn := &noopConn{}

		ctrlPort = messaging.NewPort(nil, 4, 4, "Cache.Control")
		conn.PlugIn(ctrlPort)
		topPort = messaging.NewPort(nil, 4, 4, "Cache.Top")
		conn.PlugIn(topPort)
		bottomPort = messaging.NewPort(nil, 4, 4, "Cache.Bottom")
		conn.PlugIn(bottomPort)

		initialState := State{
			DirBuf:        queueing.NewBuffer[int]("Cache.DirBuf", 4),
			BankBufs:      []queueing.Buffer[int]{},
			DirPipeline:   queueing.NewPipeline[int](4, 2),
			DirPostBuf:    queueing.NewBuffer[int]("Cache.DirPostBuf", 4),
			BankPipelines: []queueing.Pipeline[int]{},
			BankPostBufs:  []queueing.Buffer[int]{},
		}

		pmw = &pipelineMW{
			topPort:    topPort,
			bottomPort: bottomPort,
		}
		pmw.comp = modeling.NewBuilder[Spec, State, Resources]().
			WithEngine(nil).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				NumSets:          16,
				WayAssociativity: 4,
				Log2BlockSize:    6,
			}).
			Build("Cache")

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		pmw.comp.State = initialState

		s = &controlStage{
			ctrlPort: ctrlPort,
			pipeline: pmw,
		}
	})

	It("should do nothing if no request", func() {
		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should wait for the cache to finish transactions", func() {
		next := &pmw.comp.State
		next.Transactions = append(next.Transactions, transactionState{})

		flushReq := mem.ControlReq{Command: mem.CmdFlush}
		flushReq.ID = timing.GetIDGenerator().Generate()
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "mem.ControlReq"
		flushReq.DiscardInflight = false
		ctrlPort.Deliver(flushReq)

		// Store flush request in State instead of controlStage field
		next.HasProcessingFlush = true
		next.ProcessingFlush = flushReqState{
			MsgMeta:         flushReq.MsgMeta,
			DiscardInflight: flushReq.DiscardInflight,
			PauseAfter:      flushReq.PauseAfter,
		}

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should reset directory", func() {
		flushReq := mem.ControlReq{Command: mem.CmdFlush}
		flushReq.ID = timing.GetIDGenerator().Generate()
		flushReq.InvalidateAfter = true
		flushReq.DiscardInflight = true
		flushReq.PauseAfter = true
		flushReq.TrafficBytes = 0
		flushReq.TrafficClass = "mem.ControlReq"
		flushReq.Src = messaging.RemotePort("Agent")
		ctrlPort.Deliver(flushReq)

		// Store flush request in State instead of controlStage field
		next := &pmw.comp.State
		next.HasProcessingFlush = true
		next.ProcessingFlush = flushReqState{
			MsgMeta:         flushReq.MsgMeta,
			DiscardInflight: flushReq.DiscardInflight,
			PauseAfter:      flushReq.PauseAfter,
		}

		madeProgress := s.Tick()

		Expect(madeProgress).To(BeTrue())
		next = &pmw.comp.State
		Expect(next.HasProcessingFlush).To(BeFalse())

		rsp := ctrlPort.RetrieveOutgoing()
		Expect(rsp.Meta().RspTo).To(Equal(flushReq.ID))
	})

})
