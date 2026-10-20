package writeback

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/stretchr/testify/require"
)

func TestTopParser(t *testing.T) {
	var (
		m       *pipelineMW
		parser  *topParser
		topPort messaging.Port
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
		topPort = comp.Ports.Top

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState

		parser = m.topParser
	}

	t.Run("should return if no req to parse", func(t *testing.T) {
		setup1(t)

		ret := parser.Tick()
		require.False(t, ret)
	})

	t.Run("should return if the cache is not in running stage", func(t *testing.T) {
		setup1(t)

		next := &m.comp.State
		next.CacheState = int(cacheStateFlushing)

		ret := parser.Tick()
		require.False(t, ret)
	})

	t.Run("should parse read from top", func(t *testing.T) {
		setup1(t)

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x100,
			AccessByteSize: 64},
			ID: m.comp.NewID(),

			TrafficBytes: 12,
		}

		topPort.Deliver(read)

		parser.Tick()

		next := &m.comp.State
		require.Len(t, next.Transactions, 1)
		require.True(t, next.Transactions[0].HasRead)
		require.Equal(t, uint64(0x100), next.Transactions[0].ReadAddress)
		require.Equal(t, uint64(64), next.Transactions[0].ReadAccessByteSize)
		_, present0 := topPort.PeekIncoming()
		require.False(t, present0)
	})

	t.Run("should parse write from top", func(t *testing.T) {
		setup1(t)

		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x100},
			ID: m.comp.NewID(),

			TrafficBytes: 12,
		}

		topPort.Deliver(write)

		parser.Tick()

		next := &m.comp.State
		require.Len(t, next.Transactions, 1)
		require.True(t, next.Transactions[0].HasWrite)
		require.Equal(t, uint64(0x100), next.Transactions[0].WriteAddress)
		_, present1 := topPort.PeekIncoming()
		require.False(t, present1)
	})
}
