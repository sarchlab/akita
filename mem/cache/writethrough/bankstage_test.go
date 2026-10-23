package writethrough

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestBankstage(t *testing.T) {
	var (
		storage *mem.Storage
		s       *bankStage
		c       *pipelineMW
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		storage = mem.NewStorage(4 * mem.KB)

		initialState := state{
			DirBuf: queueing.MakeBuffer[int](4),
			BankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](1),
			},
			DirPipeline: queueing.MakePipeline[int](1, 2),
			DirPostBuf:  queueing.MakeBuffer[int](4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.MakePipeline[int](1, 10),
			},
			BankPostBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](1),
			},
		}

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		sim := akitasim.MakeBuilder().WithTaskTracking().Build()
		c = buildStageTestCache(sim,
			Spec{

				DirectoryWidth: 1,
				BankWidth:      1,
				Bottom: mem.AddressMapper{
					Destinations: []messaging.RemotePort{"Memory.Top"}},
				Freq:           1 * timing.GHz,
				BankLatency:    10,
				Log2BlockSize:  6,
				NumWays:        4,
				NumSets:        16,
				NumBanks:       1,
				NumReqPerCycle: 1,
				WritePolicy:    "write-around",
			},
			Resources{Storage: storage},
			makePorts(sim, "Cache", 4),
			initialState,
		)

		s = &bankStage{
			cache:          c,
			bankID:         0,
			numReqPerCycle: 1,
		}
	}

	t.Run("should do nothing if no request", func(t *testing.T) {
		setup1(t)

		madeProgress := s.Tick()

		require.False(t, madeProgress)
	})

	t.Run("should insert transactions into pipeline", func(t *testing.T) {
		setup1(t)

		next := &c.comp.State

		// Add a transaction
		next.Transactions = append(next.Transactions, transactionState{})
		next.BankBufs[0].Push(0)

		madeProgress := s.Tick()

		require.True(t, madeProgress)
		require.Len(t, next.BankPipelines[0].Stages(), 1)
	})

	t.Run("read hit", func(t *testing.T) {
		var (
			blockSetID, blockWayID int
		)

		setup2 := func(t *testing.T) {
			t.Helper()

			blockSetID = 0
			blockWayID = 0

			next := &c.comp.State

			// Set up the block in directoryState
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].ReadCount = 1
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			storage.Write(0x400, []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			})

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					BlockSetID:         blockSetID,
					BlockWayID:         blockWayID,
					HasBlock:           true,
					BankAction:         bankActionReadHit,
				},
			)

			// Put in post-pipeline buffer
			next.BankPostBufs[0].Push(0)
		}

		t.Run("should read", func(t *testing.T) {
			setup1(t)
			setup2(t)

			next := &c.comp.State

			madeProgress := s.Tick()

			require.True(t, madeProgress)
			trans := &next.Transactions[0]
			require.Equal(t, []byte{5, 6, 7, 8}, trans.Data)
			require.True(t, trans.Done)
			require.Equal(t, 0, next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].ReadCount)
		})
	})

	t.Run("write", func(t *testing.T) {
		var (
			blockSetID, blockWayID int
		)

		setup3 := func(t *testing.T) {
			t.Helper()

			blockSetID = 0
			blockWayID = 0

			next := &c.comp.State

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 64 + 12,
			}

			writeData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			writeDirtyMask := []bool{
				false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:       true,
					WriteMeta:      writeMeta,
					WriteAddress:   0x100,
					WriteData:      writeData,
					WriteDirtyMask: writeDirtyMask,
					BlockSetID:     blockSetID,
					BlockWayID:     blockWayID,
					HasBlock:       true,
					BankAction:     bankActionWrite,
				},
			)

			next.BankPostBufs[0].Push(0)
		}

		t.Run("should write", func(t *testing.T) {
			setup1(t)
			setup3(t)

			next := &c.comp.State

			madeProgress := s.Tick()

			require.True(t, madeProgress)
			require.False(t, next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked)
			data := storage.Read(0x400, 64)
			require.Equal(t, []byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				1, 2, 3, 4, 5, 6, 7, 8,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			}, data)
		})
	})

	t.Run("write fetched", func(t *testing.T) {
		var (
			blockSetID, blockWayID int
		)

		setup4 := func(t *testing.T) {
			t.Helper()

			blockSetID = 0
			blockWayID = 0

			next := &c.comp.State

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].CacheAddress = 0x400
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked = true
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			transData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					BlockSetID:            blockSetID,
					BlockWayID:            blockWayID,
					HasBlock:              true,
					BankAction:            bankActionWriteFetched,
					Data:                  transData,
					WriteFetchedDirtyMask: make([]bool, 64),
				},
			)

			next.BankPostBufs[0].Push(0)
		}

		t.Run("should write fetched", func(t *testing.T) {
			setup1(t)
			setup4(t)

			next := &c.comp.State

			madeProgress := s.Tick()

			require.True(t, madeProgress)
			require.False(t, next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsLocked)
			trans := &next.Transactions[0]
			data := storage.Read(0x400, 64)
			require.Equal(t, trans.Data, data)
		})

		t.Run("with an MSHR-coalesced write waiter", func(t *testing.T) {
			var coalescedWriteMeta messaging.Msg

			setup5 := func(t *testing.T) {
				t.Helper()

				next := &c.comp.State

				// Fetcher is the read at index 0 (set up above). Append
				// the coalesced write that depends on the fetcher's
				// merged fill landing in storage.
				coalescedWriteMeta = messaging.Msg{
					ID:           c.comp.NewID(),
					TrafficBytes: 4 + 12,
				}
				next.Transactions = append(next.Transactions,
					transactionState{
						HasWrite:           true,
						WriteMeta:          coalescedWriteMeta,
						WriteAddress:       0x108,
						WriteData:          []byte{9, 9, 9, 9},
						WritePID:           1,
						HasWriteToBottom:   true,
						WaitForMSHRFill:    true,
						MSHRFillFetcherIdx: 0,
					},
				)
			}

			t.Run("does not complete the coalesced write when its bottom ack hasn't arrived", func(t *testing.T) {
				setup1(t)
				setup4(t)
				setup5(t)

				next := &c.comp.State

				madeProgress := s.Tick()

				require.True(t, madeProgress)
				coalesced := &next.Transactions[1]
				require.True(t, coalesced.MSHRFillDone)
				require.False(t, coalesced.Done)
			})

			t.Run("completes the coalesced write when its bottom ack already arrived", func(t *testing.T) {
				setup1(t)
				setup4(t)
				setup5(t)

				next := &c.comp.State
				next.Transactions[1].BottomWriteDone = true

				madeProgress := s.Tick()

				require.True(t, madeProgress)
				coalesced := &next.Transactions[1]
				require.True(t, coalesced.MSHRFillDone)
				require.True(t, coalesced.Done)
			})
		})
	})
}
