package writethrough

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestBottomParser(t *testing.T) {
	var (
		bottomPort messaging.Port
		p          *bottomParser
		c          *pipelineMW
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		initialState := state{
			DirBuf: queueing.MakeBuffer[int](4),
			BankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			DirPipeline: queueing.MakePipeline[int](4, 2),
			DirPostBuf:  queueing.MakeBuffer[int](4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.MakePipeline[int](4, 10),
			},
			BankPostBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
		}

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		sim := akitasim.MakeBuilder().WithTaskTracking().Build()
		ports := makePorts(sim, "Cache", 4)
		c = buildStageTestCache(sim,
			Spec{Bottom: mem.AddressMapper{Destinations: []messaging.RemotePort{"Memory.Top"}},
				Freq:             1 * timing.GHz,
				Log2BlockSize:    6,
				WayAssociativity: 4,
				NumMSHREntry:     4,
				TotalByteSize:    4 * mem.KB,
				NumBanks:         1,
				WritePolicyType:  "write-around",
			},
			Resources{Storage: mem.NewStorage(4 * mem.KB)},
			ports,
			initialState,
		)

		// The bottomParser reads responses from the cache's real Bottom port,
		// so the tests deliver into it.
		bottomPort = ports.Bottom

		p = &bottomParser{cache: c}
	}

	t.Run("should do nothing if no respond", func(t *testing.T) {
		setup1(t)

		madeProgress := p.Tick()
		require.False(t, madeProgress)
	})

	t.Run("write done", func(t *testing.T) {
		t.Run("should handle write done", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeToBottomMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
			}

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
			}

			// Transaction (index 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:          true,
					WriteMeta:         writeMeta,
					WriteAddress:      0x100,
					WritePID:          1,
					HasWriteToBottom:  true,
					WriteToBottomMeta: writeToBottomMeta,
					WriteToBottomPID:  1,
				},
			)

			done := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:           c.comp.NewID(),
				RspTo:        writeToBottomMeta.ID,
				TrafficBytes: 4,
			}

			bottomPort.Deliver(done)

			madeProgress := p.Tick()

			require.True(t, madeProgress)
			require.True(t, next.Transactions[0].Done)
			// Transaction stays until bankDone is also true.
			require.True(t, next.Transactions[0].BottomWriteDone)
			require.False(t, next.Transactions[0].Removed)
		})

		t.Run("does not complete an MSHR-coalesced write before the fill", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeToBottomMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
			}
			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 4 + 12,
			}

			// Index 0 is a placeholder fetcher (the read miss that
			// allocated the MSHR). Only its index matters here.
			next.Transactions = append(next.Transactions, transactionState{})
			// Index 1 is the MSHR-coalesced write whose bottom ack arrives.
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:           true,
					WriteMeta:          writeMeta,
					WriteAddress:       0x104,
					WriteData:          []byte{1, 2, 3, 4},
					WritePID:           1,
					HasWriteToBottom:   true,
					WriteToBottomMeta:  writeToBottomMeta,
					WriteToBottomPID:   1,
					WaitForMSHRFill:    true,
					MSHRFillFetcherIdx: 0,
				},
			)

			done := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
				ID:           c.comp.NewID(),
				RspTo:        writeToBottomMeta.ID,
				TrafficBytes: 4,
			}

			bottomPort.Deliver(done)

			madeProgress := p.Tick()

			require.True(t, madeProgress)
			coalesced := &next.Transactions[1]
			require.True(t, coalesced.BottomWriteDone)
			// Must NOT be done — the fetcher's bankActionWriteFetched
			// stage hasn't written the merged line yet.
			require.False(t, coalesced.Done)
		})
	})

	t.Run("data ready", func(t *testing.T) {
		var (
			readToBottomMeta       messaging.Msg
			dataReady              messaging.Msg
			blockSetID, blockWayID int
		)

		setup2 := func(t *testing.T) {
			t.Helper()

			next := &c.comp.State

			readToBottomMeta = messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
			}

			drData := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			dataReady = messaging.Msg{Payload: memprotocol.DataReadyRsp{
				Data: drData},
				ID:    c.comp.NewID(),
				RspTo: readToBottomMeta.ID,

				TrafficBytes: len(drData) + 4,
			}

			blockSetID = 4
			blockWayID = 0

			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].PID = 1
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].Tag = 0x100
			next.DirectoryState.Sets[blockSetID].Blocks[blockWayID].IsValid = true

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
			}

			// Read transaction (index 0) — the fetcher
			next.Transactions = append(next.Transactions,
				transactionState{
					BlockSetID:         blockSetID,
					BlockWayID:         blockWayID,
					HasBlock:           true,
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x100,
					ReadAccessByteSize: 4,
					ReadPID:            1,
					HasReadToBottom:    true,
					ReadToBottomMeta:   readToBottomMeta,
					ReadToBottomPID:    1,
				},
			)

			// Set up MSHR entry with block reference and transaction 0
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			entry := &next.MSHRState.Entries[entryIdx]
			entry.HasBlock = true
			entry.BlockSetID = blockSetID
			entry.BlockWayID = blockWayID
			entry.TransactionIndices = append(entry.TransactionIndices, 0)
		}

		t.Run("should stall if bank is busy", func(t *testing.T) {
			setup1(t)
			setup2(t)

			next := &c.comp.State
			next.BankBufs[0] = queueing.MakeBuffer[int](0)

			bottomPort.Deliver(dataReady)

			madeProgress := p.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should send transaction to bank", func(t *testing.T) {
			setup1(t)
			setup2(t)

			next := &c.comp.State

			bottomPort.Deliver(dataReady)

			madeProgress := p.Tick()

			require.True(t, madeProgress)
			// Fetcher is NOT marked Done yet — bank stage does that
			require.False(t, next.Transactions[0].Done)
			// Fetcher's Data holds the full block for bank write-fetched
			require.Equal(t, []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}, next.Transactions[0].Data)
			// Bank buf should have a write-fetched transaction
			require.Equal(t, 1, next.BankBufs[0].Size())
			// Fetcher trans should have bankAction set
			trans := &next.Transactions[0]
			require.Equal(t, bankActionWriteFetched, trans.BankAction)
		})

		t.Run("should combine write", func(t *testing.T) {
			setup1(t)
			setup2(t)

			next := &c.comp.State

			// Add another read transaction (index 1) that is in the MSHR
			read2Meta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           read2Meta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)

			// Add a write transaction (index 2)
			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 16 + 12,
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x100,
					WritePID:     1,
					WriteData: []byte{
						0, 0, 0, 0, 0, 0, 0, 0,
						9, 9, 9, 9, 9, 9, 9, 9,
					},
					WriteDirtyMask: []bool{
						false, false, false, false, false, false, false, false,
						true, true, true, true, true, true, true, true,
					},
				},
			)

			entryIdx, _ := cache.MSHRQuery(&next.MSHRState, vm.PID(1), uint64(0x100))
			next.MSHRState.Entries[entryIdx].TransactionIndices = append(
				next.MSHRState.Entries[entryIdx].TransactionIndices, 1, 2)

			bottomPort.Deliver(dataReady)

			madeProgress := p.Tick()

			require.True(t, madeProgress)
			// Fetcher's Data holds the full merged block (not the 4-byte
			// read slice — that's restored by the bank stage).
			// Fetcher is NOT marked Done yet — bank stage does that.
			require.False(t, next.Transactions[0].Done)
			trans := &next.Transactions[0]
			require.Equal(t, bankActionWriteFetched, trans.BankAction)
			require.Equal(t, []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				9, 9, 9, 9, 9, 9, 9, 9,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}, trans.Data)
			// Other transactions finalized
			require.True(t, next.Transactions[1].Done)
			require.Equal(t, []byte{5, 6, 7, 8}, next.Transactions[1].Data)
			require.True(t, next.Transactions[2].Done)
			// Bank buf should have the fetcher transaction
			require.Equal(t, 1, next.BankBufs[0].Size())
		})
	})
}
