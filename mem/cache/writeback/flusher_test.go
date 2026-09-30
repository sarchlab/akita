package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
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
			BankPostPipelineBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("BankPostPipelineBuf", 4),
			},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		spec := stageTestSpec()
		comp := buildStageTestComp(spec,
			Resources{Storage: mem.NewStorage(spec.TotalByteSize)},
			makePorts("Cache", 4))
		controlPort = comp.Ports.Control

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		f = comp.Middlewares.Flusher.flusher
	})

	It("should do nothing if no request", func() {
		ret := f.Tick()
		Expect(ret).To(BeFalse())
	})

	Context("flush without reset", func() {
		It("should start flushing", func() {
			// Flush is a conditional verb: it is only legal once paused.
			m.comp.State.CacheState = int(cacheStatePaused)

			req := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush}
			req.ID = m.comp.NewID()
			req.TrafficClass = "memcontrolprotocol.Req"
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
					ID: m.comp.NewID(),
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
					ID: m.comp.NewID(),
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
			flushID := m.comp.NewID()
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

			out, _ := controlPort.RetrieveOutgoing()
			Expect(out).NotTo(BeNil())
			Expect(out.Meta().RspTo).To(Equal(flushID))
		})
	})

	Context("flush with reset", func() {
		It("should remove inflight state", func() {
			// Flush is a conditional verb: it is only legal once paused.
			m.comp.State.CacheState = int(cacheStatePaused)

			req := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush}
			req.ID = m.comp.NewID()
			req.TrafficClass = "memcontrolprotocol.Req"

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
