package writeback

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestBankStage(t *testing.T) {
	var (
		m       *pipelineMW
		bs      *bankStage
		storage *mem.Storage
		topPort messaging.Port
	)

	// fillTop pre-fills topPort's single outgoing slot so the next CanSend
	// returns false, simulating a busy port.
	fillTop := func(t *testing.T) {
		t.Helper()

		dummy := messaging.Msg{Payload: memprotocol.DataReadyRsp{},
			Src:          topPort.AsRemote(),
			Dst:          messaging.RemotePort("SomeSrc"),
			TrafficClass: "rsp"}

		require.True(t, topPort.CanSend())
		topPort.Send(dummy)
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		storage = mem.NewStorage(4 * mem.KB)

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
		spec.BankLatency = 10

		// The stage sends on the Top port, so the test gives it a single-slot
		// Top port to simulate a busy port.
		ports := makePorts("Cache", 4)
		ports.Top = twowaybuffered.NewPort(1, 1)
		comp := buildStageTestComp(spec, Resources{Storage: storage}, ports)
		topPort = comp.Ports.Top

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState
		next := &m.comp.State

		cache.DirectoryReset(&next.DirectoryState, 64, 4, 64)

		bs = m.bankStages[0]
	}

	t.Run("completes a later transaction while preserving a blocked zero-index head", func(t *testing.T) {
		setup1(t)

		next := &m.comp.State
		fillTop(t)
		next.Transactions = []transactionState{
			{Action: bankReadHit},
			{Action: bankEvict},
			{Action: bankWriteHit},
		}
		for _, idx := range []int{0, 1, 2} {
			next.BankPostPipelineBufs[0].Push(idx)
		}
		next.BankInflightTransCounts[0] = 3
		next.BankDownwardInflightTransCounts[0] = 1

		require.True(t, bs.finalizeTrans())
		require.Equal(t, []int{0, 2}, next.BankPostPipelineBufs[0].Elements())
		require.Equal(t, []int{1}, next.WriteBufferBuf.Elements())
		require.False(t, next.Transactions[0].Removed)
		require.Equal(t, writeBufferFlush, next.Transactions[1].Action)
		require.Equal(t, 2, next.BankInflightTransCounts[0])
		require.Equal(t, 0, next.BankDownwardInflightTransCounts[0])
		require.False(t, bs.finalizeTrans())
		require.Equal(t, []int{0, 2}, next.BankPostPipelineBufs[0].Elements())
	})

	t.Run("completing a read hit transaction", func(t *testing.T) {
		setup2 := func(t *testing.T) {
			t.Helper()

			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1

			storage.Write(0x40, []byte{1, 2, 3, 4, 5, 6, 7, 8})

			read := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        0x104,
				AccessByteSize: 4},
				ID:  m.comp.NewID(),
				Src: messaging.RemotePort("Agent"),

				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq"}

			trans := transactionState{
				HasRead:            true,
				ReadMeta:           read,
				ReadAddress:        read.Payload.(memprotocol.ReadReq).Address,
				ReadAccessByteSize: read.Payload.(memprotocol.ReadReq).AccessByteSize,
				ReadPID:            read.Payload.(memprotocol.ReadReq).PID,
				BlockSetID:         0,
				BlockWayID:         0,
				HasBlock:           true,
				Action:             bankReadHit,
			}
			next.Transactions = []transactionState{trans}

			// Put transaction in bank post-pipeline buffer
			next.BankPostPipelineBufs[0].Push(0)
			next.BankInflightTransCounts[0] = 1
		}

		t.Run("should stall if send buffer is full", func(t *testing.T) {
			setup1(t)
			setup2(t)

			fillTop(t)

			ret := bs.Tick()

			require.False(t, ret)
			next := &m.comp.State
			require.Equal(t, 1, next.BankInflightTransCounts[0])
		})

		t.Run("should read and send response", func(t *testing.T) {
			setup1(t)
			setup2(t)

			ret := bs.Tick()

			require.True(t, ret)
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			require.Equal(t, 0, block.ReadCount)
			require.True(t, next.Transactions[0].Removed)
			require.Equal(t, 0, next.BankInflightTransCounts[0])

			out, _ := topPort.RetrieveOutgoing()
			dr := out
			require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
		})
	})

	t.Run("completing a write-hit transaction", func(t *testing.T) {
		setup3 := func(t *testing.T) {
			t.Helper()

			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1
			block.IsLocked = true

			write := messaging.Msg{Payload: memprotocol.WriteReq{
				Address: 0x104,
				Data:    []byte{5, 6, 7, 8}},
				ID:  m.comp.NewID(),
				Src: messaging.RemotePort("Agent"),

				TrafficBytes: len([]byte{5, 6, 7, 8}) + 12,
				TrafficClass: "memprotocol.WriteReq"}

			trans := transactionState{
				HasWrite:     true,
				WriteMeta:    write,
				WriteAddress: write.Payload.(memprotocol.WriteReq).Address,
				WriteData:    write.Payload.(memprotocol.WriteReq).Data,
				WritePID:     write.Payload.(memprotocol.WriteReq).PID,
				BlockSetID:   0,
				BlockWayID:   0,
				HasBlock:     true,
				Action:       bankWriteHit,
			}
			next.Transactions = []transactionState{trans}
			next.BankPostPipelineBufs[0].Push(0)
			next.BankInflightTransCounts[0] = 1
		}

		t.Run("should stall if send buffer is full", func(t *testing.T) {
			setup1(t)
			setup3(t)

			fillTop(t)

			ret := bs.Tick()

			require.False(t, ret)
			next := &m.comp.State
			require.Equal(t, 1, next.BankInflightTransCounts[0])
		})

		t.Run("should write and send response", func(t *testing.T) {
			setup1(t)
			setup3(t)

			ret := bs.Tick()

			require.True(t, ret)
			data := storage.Read(0x44, 4)
			require.Equal(t, []byte{5, 6, 7, 8}, data)
			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			require.True(t, block.IsValid)
			require.False(t, block.IsLocked)
			require.True(t, block.IsDirty)
			require.True(t, next.Transactions[0].Removed)
			require.Equal(t, 0, next.BankInflightTransCounts[0])

			out, _ := topPort.RetrieveOutgoing()
			require.NotNil(t, out)
		})
	})

	t.Run("completing a write fetched transaction", func(t *testing.T) {
		setup4 := func(t *testing.T) {
			t.Helper()

			next := &m.comp.State
			block := &next.DirectoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.IsLocked = true

			fetchedData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}

			trans := transactionState{
				BlockSetID:             0,
				BlockWayID:             0,
				HasBlock:               true,
				MSHRData:               fetchedData,
				MSHRTransactionIndices: []int{},
				Action:                 bankWriteFetched,
			}
			next.Transactions = []transactionState{trans}
			next.BankPostPipelineBufs[0].Push(0)
			next.BankInflightTransCounts[0] = 1
		}

		t.Run("should write to storage and send to mshr stage", func(t *testing.T) {
			setup1(t)
			setup4(t)

			ret := bs.Tick()

			require.True(t, ret)
			next := &m.comp.State
			writtenData := storage.Read(0x40, 64)
			require.Equal(t, next.Transactions[0].MSHRData, writtenData)
			block := &next.DirectoryState.Sets[0].Blocks[0]
			require.False(t, block.IsLocked)
			require.True(t, block.IsValid)
			require.Equal(t, 0, next.BankInflightTransCounts[0])
		})
	})

	t.Run("finalizing a read for eviction action", func(t *testing.T) {
		setup5 := func(t *testing.T) {
			t.Helper()

			next := &m.comp.State
			trans := transactionState{
				HasVictim:          true,
				VictimTag:          0x200,
				VictimCacheAddress: 0x300,
				VictimDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
				Action:       bankEvictAndFetch,
				EvictingAddr: 0x200,
			}
			next.Transactions = []transactionState{trans}
			next.BankPostPipelineBufs[0].Push(0)
			next.BankInflightTransCounts[0] = 1
		}

		t.Run("should send eviction to write buffer", func(t *testing.T) {
			setup1(t)
			setup5(t)

			data := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			storage.Write(0x300, data)

			ret := bs.Tick()

			require.True(t, ret)
			next := &m.comp.State
			require.Equal(t, writeBufferEvictAndFetch, next.Transactions[0].Action)
			require.Equal(t, data, next.Transactions[0].EvictingData)
			require.Equal(t, 0, next.BankInflightTransCounts[0])
		})
	})
}
