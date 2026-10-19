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

func TestWriteBufferStage(t *testing.T) {
	var (
		m          *pipelineMW
		wb         *writeBufferStage
		bottomPort messaging.Port
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
		spec.WriteBufferCapacity = 16
		spec.MaxInflightFetch = 4
		spec.MaxInflightEviction = 4
		spec.Bottom.Destinations = []messaging.RemotePort{"DRAM"}
		sim := akitasim.MakeBuilder().WithTaskTracking().Build()
		comp := buildStageTestComp(sim, spec,
			Resources{
				Storage: mem.NewStorage(spec.TotalByteSize),
			},
			makePorts(sim, "Cache", 4))
		bottomPort = comp.Ports.Bottom

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState

		wb = m.writeBuffer
	}

	t.Run("should do nothing if no transactions", func(t *testing.T) {
		setup1(t)

		ret := wb.Tick()

		require.False(t, ret)
	})

	t.Run("processing new writeBufferFetch transactions", func(t *testing.T) {
		t.Run("should fetch from bottom", func(t *testing.T) {
			setup1(t)

			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID: m.comp.NewID(),
			}

			trans := transactionState{
				Action:       writeBufferFetch,
				FetchAddress: 0x100,
				FetchPID:     1,
				BlockSetID:   0,
				BlockWayID:   0,
				HasBlock:     true,
				HasRead:      true,
				ReadMeta:     read,
				ReadAddress:  0x100,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.WriteBufferBuf.Clear()
			next.WriteBufferBuf.Push(0)

			ret := wb.Tick()

			require.True(t, ret)
			next = &m.comp.State
			require.Len(t, next.InflightFetchIndices, 1)

			out, _ := bottomPort.RetrieveOutgoing()
			require.NotNil(t, out)
		})

		t.Run("should stall fetch if too many inflight fetches", func(t *testing.T) {
			setup1(t)

			next := &m.comp.State
			next.InflightFetchIndices = []int{10, 11, 12, 13}

			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID: m.comp.NewID(),
			}

			trans := transactionState{
				Action:       writeBufferFetch,
				FetchAddress: 0x100,
				HasRead:      true,
				ReadMeta:     read,
				ReadAddress:  0x100,
			}
			next.Transactions = []transactionState{trans}
			next.WriteBufferBuf.Clear()
			next.WriteBufferBuf.Push(0)

			ret := wb.Tick()

			require.False(t, ret)
			_, present0 := bottomPort.PeekOutgoing()
			require.False(t, present0)
		})
	})

	t.Run("writing evictions", func(t *testing.T) {
		t.Run("should send eviction to bottom", func(t *testing.T) {
			setup1(t)

			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID: m.comp.NewID(),
			}

			trans := transactionState{
				EvictingAddr: 0x200,
				EvictingPID:  2,
				EvictingData: make([]byte, 64),
				HasRead:      true,
				ReadMeta:     read,
				ReadAddress:  0x200,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.PendingEvictionIndices = []int{0}

			ret := wb.Tick()

			require.True(t, ret)
			next = &m.comp.State
			require.Len(t, next.PendingEvictionIndices, 0)
			require.Len(t, next.InflightEvictionIndices, 1)

			out, _ := bottomPort.RetrieveOutgoing()
			require.NotNil(t, out)
		})

		t.Run("should stall if too many inflight evictions", func(t *testing.T) {
			setup1(t)

			next := &m.comp.State
			next.InflightEvictionIndices = []int{10, 11, 12, 13}
			next.PendingEvictionIndices = []int{0}
			next.Transactions = []transactionState{{}}

			ret := wb.Tick()

			require.False(t, ret)
			_, present1 := bottomPort.PeekOutgoing()
			require.False(t, present1)
		})
	})

	t.Run("processing responses", func(t *testing.T) {
		t.Run("should process write done response", func(t *testing.T) {
			setup1(t)

			evictWrite := messaging.Msg{Payload: memprotocol.WriteReq{},
				ID: 9001,
			}

			read := messaging.Msg{Payload: memprotocol.ReadReq{},
				ID: m.comp.NewID(),
			}

			trans := transactionState{
				HasEvictionWriteReq:  true,
				EvictionWriteReqMeta: evictWrite,
				HasRead:              true,
				ReadMeta:             read,
				ReadAddress:          0,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.InflightEvictionIndices = []int{0}

			rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				RspTo: 9001,
			}

			bottomPort.Deliver(rsp)

			ret := wb.Tick()

			require.True(t, ret)
			next = &m.comp.State
			require.Len(t, next.InflightEvictionIndices, 0)
			_, present2 := bottomPort.PeekIncoming()
			require.False(t, present2)
		})
	})
}
