package writeback

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/stretchr/testify/require"
)

func TestDirectoryStage(t *testing.T) {
	var (
		ds *directoryStage
		m  *pipelineMW
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
		spec.NumMSHREntries = 16
		sim := akitasim.MakeBuilder().WithTaskTracking().Build()
		comp := buildStageTestComp(sim, spec,
			Resources{Storage: mem.NewStorage(spec.ByteSize())},
			makePorts(sim, "Cache", 4))

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		ds = m.dirStage
	}

	t.Run("read", func(t *testing.T) {
		setup3 := func(t *testing.T) {
			t.Helper()

			read := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x100,
				PID:            1,
				AccessByteSize: 64},
				ID: m.comp.NewID(),

				TrafficBytes: 12,
			}

			trans := transactionState{
				HasRead:            true,
				ReadMeta:           read,
				ReadAddress:        read.Payload.(memprotocol.ReadReq).Address,
				ReadAccessByteSize: read.Payload.(memprotocol.ReadReq).AccessByteSize,
				ReadPID:            read.Payload.(memprotocol.ReadReq).PID,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.DirPostPipelineBuf.Clear()
			next.DirPostPipelineBuf.Push(0)
		}

		t.Run("mshr hit", func(t *testing.T) {
			setup4 := func(t *testing.T) {
				t.Helper()

				next := &m.comp.State
				cache.MSHRAdd(&next.MSHRState, 16, vm.PID(1), 0x100)
			}

			t.Run("should add to MSHR", func(t *testing.T) {
				setup1(t)
				setup3(t)
				setup4(t)

				ret := ds.Tick()

				require.True(t, ret)
				next := &m.comp.State
				require.Len(t, next.MSHRState.Entries[0].TransactionIndices, 1)
			})
		})

		t.Run("hit", func(t *testing.T) {
			setup5 := func(t *testing.T) {
				t.Helper()

				next := &m.comp.State
				setID := cache.DirectorySetID(0x100, 64, 64)
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			}

			t.Run("should pass transaction to bank", func(t *testing.T) {
				setup1(t)
				setup3(t)
				setup5(t)

				ret := ds.Tick()

				require.True(t, ret)
				next := &m.comp.State
				setID := cache.DirectorySetID(0x100, 64, 64)
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				require.Equal(t, 1, block.ReadCount)
				require.Equal(t, bankReadHit, next.Transactions[0].Action)
			})
		})

		t.Run("miss, mshr miss, no need to evict", func(t *testing.T) {
			t.Run("should create mshr entry and fetch", func(t *testing.T) {
				setup1(t)
				setup3(t)

				ret := ds.Tick()

				require.True(t, ret)
				next := &m.comp.State
				require.Len(t, next.MSHRState.Entries, 1)
			})
		})

		t.Run("miss, mshr miss, need eviction", func(t *testing.T) {
			setup6 := func(t *testing.T) {
				t.Helper()

				next := &m.comp.State
				setID := cache.DirectorySetID(0x100, 64, 64)
				for i := range 4 {
					block := &next.DirectoryState.Sets[setID].Blocks[i]
					block.PID = 2
					block.Tag = uint64(0x200 + i*0x1000)
					block.CacheAddress = uint64(i * 64)
					block.IsValid = true
					block.IsDirty = true
				}
			}

			t.Run("should do evict", func(t *testing.T) {
				setup1(t)
				setup3(t)
				setup6(t)

				ret := ds.Tick()

				require.True(t, ret)
				next := &m.comp.State
				require.Equal(t, bankEvictAndFetch, next.Transactions[0].Action)
			})
		})
	})

	t.Run("write", func(t *testing.T) {
		setup7 := func(t *testing.T) {
			t.Helper()

			write := messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x100,
				PID:     1},
				ID: m.comp.NewID(),

				TrafficBytes: 12,
			}

			trans := transactionState{
				HasWrite:     true,
				WriteMeta:    write,
				WriteAddress: write.Payload.(memprotocol.WriteReq).Address,
				WritePID:     write.Payload.(memprotocol.WriteReq).PID,
			}

			next := &m.comp.State
			next.Transactions = []transactionState{trans}
			next.DirPostPipelineBuf.Clear()
			next.DirPostPipelineBuf.Push(0)
		}

		t.Run("hit", func(t *testing.T) {
			setup8 := func(t *testing.T) {
				t.Helper()

				next := &m.comp.State
				setID := cache.DirectorySetID(0x100, 64, 64)
				block := &next.DirectoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			}

			t.Run("should send to bank", func(t *testing.T) {
				setup1(t)
				setup7(t)
				setup8(t)

				ret := ds.Tick()

				require.True(t, ret)
				next := &m.comp.State
				require.Equal(t, bankWriteHit, next.Transactions[0].Action)
			})
		})

		t.Run("miss, write full line, no eviction", func(t *testing.T) {
			setup9 := func(t *testing.T) {
				t.Helper()

				next := &m.comp.State
				next.Transactions[0].WriteData = make([]byte, 64)
			}

			t.Run("should send to bank", func(t *testing.T) {
				setup1(t)
				setup7(t)
				setup9(t)

				ret := ds.Tick()

				require.True(t, ret)
				next := &m.comp.State
				require.Equal(t, bankWriteHit, next.Transactions[0].Action)
			})
		})
	})
}
