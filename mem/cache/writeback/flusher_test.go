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
)

var _ = Describe("Flusher", func() {
	var (
		controlPort messaging.Port
		m           *pipelineMW
		f           *flusher
	)

	BeforeEach(func() {
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

		m = &pipelineMW{}
		m.comp = modeling.NewBuilder[Spec, State, Resources]().
			WithEngine(timing.NewSerialEngine()).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumSets:          64,
				NumBanks:         1,
			}).
			Build("Cache")

		// The flusher resolves the "Control" port by name; the data pipeline
		// resolves "Top"/"Bottom". Assign real ports (owned by the component)
		// and plug a noop connection. Ticking the flusher only touches Control,
		// but Top/Bottom are declared so the pipeline can resolve them too.
		controlPort = messaging.NewPort(m.comp, 4, 4, "Cache.Control")
		(&ccNoopConn{}).PlugIn(controlPort)
		m.comp.DeclarePort("Control")
		m.comp.AssignPort("Control", controlPort)

		topPort := messaging.NewPort(m.comp, 4, 4, "Cache.Top")
		(&ccNoopConn{}).PlugIn(topPort)
		m.comp.DeclarePort("Top")
		m.comp.AssignPort("Top", topPort)

		bottomPort := messaging.NewPort(m.comp, 4, 4, "Cache.Bottom")
		(&ccNoopConn{}).PlugIn(bottomPort)
		m.comp.DeclarePort("Bottom")
		m.comp.AssignPort("Bottom", bottomPort)

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

		f = &flusher{pipeline: m}
	})

	It("should do nothing if no request", func() {
		ret := f.Tick()
		Expect(ret).To(BeFalse())
	})

	Context("flush without reset", func() {
		It("should start flushing", func() {
			// Flush is a conditional verb: it is only legal once paused.
			m.comp.State.CacheState = int(cacheStatePaused)

			req := mem.ControlReq{Command: mem.CmdFlush}
			req.ID = timing.GetIDGenerator().Generate()
			req.TrafficClass = "mem.ControlReq"
			controlPort.Deliver(req)

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
			flushID := timing.GetIDGenerator().Generate()
			next.ProcessingFlush = flushReqState{
				MsgMeta: messaging.MsgMeta{
					ID:  flushID,
					Src: messaging.RemotePort("Agent"),
				},
			}
			next.FlusherBlockToEvictRefs = []blockRef{}

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next = &m.comp.State
			Expect(next.HasProcessingFlush).To(BeFalse())
			// Flush returns the cache to paused (its prior, legal state).
			Expect(cacheState(next.CacheState)).To(Equal(cacheStatePaused))

			out := controlPort.RetrieveOutgoing()
			Expect(out).NotTo(BeNil())
			Expect(out.Meta().RspTo).To(Equal(flushID))
		})
	})

	Context("flush with reset", func() {
		It("should remove inflight state", func() {
			// Flush is a conditional verb: it is only legal once paused.
			m.comp.State.CacheState = int(cacheStatePaused)

			req := mem.ControlReq{Command: mem.CmdFlush}
			req.ID = timing.GetIDGenerator().Generate()
			req.TrafficClass = "mem.ControlReq"

			controlPort.Deliver(req)

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			next := &m.comp.State
			Expect(next.HasProcessingFlush).To(BeTrue())
			Expect(cacheState(next.CacheState)).To(Equal(cacheStatePreFlushing))
		})
	})

	// CmdEnable / Reset handling moved out of flusher to ctrlmiddleware;
	// those code paths are covered by TestControlContract.
})
