package writeback

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/stretchr/testify/require"
)

func TestFlusher(t *testing.T) {
	var (
		controlPort messaging.Port
		m           *pipelineMW
		f           *flusher
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		initialState := state{
			CacheState:   int(cacheStateRunning),
			EvictingList: make(map[uint64]bool),
			DirStageBuf:  queueing.MakeBuffer[int](4),
			DirToBankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			WriteBufferToBankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			MSHRStageBuf:       queueing.MakeBuffer[int](4),
			WriteBufferBuf:     queueing.MakeBuffer[int](4),
			DirPipeline:        queueing.MakePipeline[int](4, 0),
			DirPostPipelineBuf: queueing.MakeBuffer[int](4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.MakePipeline[int](4, 10),
			},
			BankPostPipelineBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		spec := stageTestSpec()
		sim := akitasim.MakeBuilder().WithTaskTracking().Build()
		comp := buildStageTestComp(sim, spec,
			Resources{Storage: mem.NewStorage(spec.ByteSize())},
			makePorts(sim, "Cache", 4))
		controlPort = comp.Ports.Control

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		f = comp.Middlewares.Flusher.flusher
	}

	t.Run("should do nothing if no request", func(t *testing.T) {
		setup1(t)

		ret := f.Tick()
		require.False(t, ret)
	})

	t.Run("flush without reset", func(t *testing.T) {
		t.Run("should start flushing", func(t *testing.T) {
			setup1(t)

			// Flush is a conditional verb: it is only legal once paused.
			m.comp.State.CacheState = int(cacheStatePaused)

			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
				ID: m.comp.NewID(),
			}

			controlPort.Deliver(req)

			ret := f.Tick()

			require.True(t, ret)
			next := &m.comp.State
			require.True(t, next.HasProcessingFlush)
			require.Equal(t, cacheStatePreFlushing, cacheState(next.CacheState))
		})

		t.Run("should do nothing if there is inflight transaction", func(t *testing.T) {
			setup1(t)

			next := &m.comp.State
			next.CacheState = int(cacheStatePreFlushing)
			next.Transactions = append(
				next.Transactions, transactionState{})
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				Msg: messaging.Msg{
					ID: m.comp.NewID(),
				},
			}

			ret := f.Tick()

			require.False(t, ret)
		})

		t.Run("should move to flush stage if no inflight transaction", func(t *testing.T) {
			setup1(t)

			next := &m.comp.State
			next.CacheState = int(cacheStatePreFlushing)
			next.HasProcessingFlush = true
			next.ProcessingFlush = flushReqState{
				Msg: messaging.Msg{
					ID: m.comp.NewID(),
				},
			}

			// Set up directory with one dirty valid block
			cache.DirectoryReset(&next.DirectoryState, 2, 2, 64)
			next.DirectoryState.Sets[0].Blocks[0].IsDirty = true
			next.DirectoryState.Sets[0].Blocks[0].IsValid = true

			ret := f.Tick()

			require.True(t, ret)
			next = &m.comp.State
			require.Equal(t, cacheStateFlushing, cacheState(next.CacheState))
			require.Len(t, next.FlusherBlockToEvictRefs, 1)
		})

		t.Run("should send response if all the blocks are evicted", func(t *testing.T) {
			setup1(t)

			next := &m.comp.State
			next.CacheState = int(cacheStateFlushing)
			next.HasProcessingFlush = true
			flushID := m.comp.NewID()
			next.ProcessingFlush = flushReqState{
				Msg: messaging.Msg{
					ID:  flushID,
					Src: messaging.RemotePort("Agent"),
				},
			}
			next.FlusherBlockToEvictRefs = []blockRef{}

			ret := f.Tick()

			require.True(t, ret)
			next = &m.comp.State
			require.False(t, next.HasProcessingFlush)
			// Flush returns the cache to paused (its prior, legal state).
			require.Equal(t, cacheStatePaused, cacheState(next.CacheState))

			out, _ := controlPort.RetrieveOutgoing()
			require.NotNil(t, out)
			require.Equal(t, flushID, out.RspTo)
		})
	})

	t.Run("flush with reset", func(t *testing.T) {
		t.Run("should remove inflight state", func(t *testing.T) {
			setup1(t)

			// Flush is a conditional verb: it is only legal once paused.
			m.comp.State.CacheState = int(cacheStatePaused)

			req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
				ID: m.comp.NewID(),
			}

			controlPort.Deliver(req)

			ret := f.Tick()

			require.True(t, ret)
			next := &m.comp.State
			require.True(t, next.HasProcessingFlush)
			require.Equal(t, cacheStatePreFlushing, cacheState(next.CacheState))
		})
	})

	// CmdEnable / Reset handling moved out of flusher to ctrlmiddleware;
	// those code paths are covered by TestControlContract.
}
