package writethrough

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/queueing"

	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestDirectory(t *testing.T) {
	var (
		bottomPort messaging.Port
		d          *directory
		c          *pipelineMW
	)

	// fillBottomOutgoing pre-fills bottomPort's single outgoing slot so the
	// next CanSend returns false, simulating a busy port.
	fillBottomOutgoing := func(t *testing.T) {
		t.Helper()

		dummy := messaging.Msg{Payload: memprotocol.ReadReq{},
			ID:           c.comp.NewID(),
			Src:          bottomPort.AsRemote(),
			Dst:          messaging.RemotePort("DRAM"),
			TrafficClass: "req"}

		require.True(t, bottomPort.CanSend())
		bottomPort.Send(dummy)
	}

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

		// bottomPort is a real, single-slot port owned by the component.
		// Success cases read the sent request back via RetrieveOutgoing;
		// failure cases pre-fill the slot.
		ports := makePorts("Cache", 4)
		ports.Bottom = twowaybuffered.NewPort(1, 1)

		c = buildStageTestCache(
			Spec{
				Freq:             1 * timing.GHz,
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumMSHREntry:     4,
				TotalByteSize:    4 * mem.KB, // 16 sets
				NumBanks:         1,
				WritePolicyType:  "write-around",
				Bottom:           mem.AddressMapper{Destinations: []messaging.RemotePort{"DRAM"}},
			},
			Resources{
				Storage: mem.NewStorage(4 * mem.KB),
			},
			ports,
			initialState,
		)

		bottomPort = ports.Bottom

		d = &directory{
			cache: c,
		}
	}

	t.Run("should do nothing if no transaction", func(t *testing.T) {
		setup1(t)

		madeProgress := d.Tick()

		require.False(t, madeProgress)
	})

	t.Run("read mshr hit", func(t *testing.T) {
		t.Run("Should add to mshr entry", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Transaction (idx 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)

			// Pre-populate MSHR with an entry
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))

			// Put trans in post-pipeline buffer
			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()

			require.True(t, madeProgress)
			entry := next.MSHRState.Entries[entryIdx]
			require.Contains(t, entry.TransactionIndices, 0)
		})
	})

	t.Run("read hit", func(t *testing.T) {
		t.Run("should send transaction to bank", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			// Transaction (idx 0)
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)

			// Set up a valid block in directory at the right set for address 0x100
			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()

			trans := &next.Transactions[0]
			require.True(t, madeProgress)
			require.True(t, trans.HasBlock)
			require.Equal(t, setID, trans.BlockSetID)
			require.Equal(t, wayID, trans.BlockWayID)
			require.Equal(t, bankActionReadHit, trans.BankAction)
			require.Equal(t, 1, next.DirectoryState.Sets[setID].Blocks[wayID].ReadCount)
			// Bank buf should have the trans index
			require.Equal(t, 1, next.BankBufs[0].Size())
		})

		t.Run("should stall if cannot send to bank", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)

			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.Push(0)

			// Fill up bank buffer
			next.BankBufs[0] = queueing.MakeBuffer[int](0)

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should stall if block is locked", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)

			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked = true

			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()
			require.False(t, madeProgress)
		})
	})

	t.Run("read miss", func(t *testing.T) {
		t.Run("should send request to bottom", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()

			require.True(t, madeProgress)

			readToBottomValue, _ := bottomPort.RetrieveOutgoing()

			readToBottom := readToBottomValue
			require.Equal(t, uint64(0x100), readToBottom.Payload.(memprotocol.ReadReq).Address)
			require.Equal(t, uint64(64), readToBottom.Payload.(memprotocol.ReadReq).AccessByteSize)
			require.Equal(t, vm.PID(1), readToBottom.Payload.(memprotocol.ReadReq).PID)
			// Check MSHR entry was created
			entryIdx, found := cache.MSHRQuery(&next.MSHRState, vm.PID(1), 0x100)
			require.True(t, found)
			entry := next.MSHRState.Entries[entryIdx]
			require.Contains(t, entry.TransactionIndices, 0)
			require.True(t, entry.HasBlock)
			require.True(t, entry.HasReadReq)

			// Check victim block was set up
			victimSetID := entry.BlockSetID
			victimWayID := entry.BlockWayID
			block := next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
			require.Equal(t, uint64(0x100), block.Tag)
			require.True(t, block.IsLocked)
			require.True(t, block.IsValid)
			trans := &next.Transactions[0]
			require.True(t, trans.HasReadToBottom)
			require.True(t, trans.HasBlock)
		})

		t.Run("should stall if every way in the set is locked", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
			next.DirPostBuf.Push(0)

			setID := cache.DirectorySetID(0x100, 64, 16)
			for w := range next.DirectoryState.Sets[setID].Blocks {
				next.DirectoryState.Sets[setID].Blocks[w].IsLocked = true
			}

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should stall if every way in the set is being read", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
			next.DirPostBuf.Push(0)

			setID := cache.DirectorySetID(0x100, 64, 16)
			for w := range next.DirectoryState.Sets[setID].Blocks {
				next.DirectoryState.Sets[setID].Blocks[w].ReadCount = 1
			}

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should skip the LRU victim if it is locked and try another way", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
			next.DirPostBuf.Push(0)

			setID := cache.DirectorySetID(0x100, 64, 16)
			// Lock only the LRU-most way; other ways should still be picked.
			lockedWay := next.DirectoryState.Sets[setID].LRUOrder[0]
			next.DirectoryState.Sets[setID].Blocks[lockedWay].IsLocked = true

			madeProgress := d.Tick()

			require.True(t, madeProgress)
			trans := &next.Transactions[0]
			require.True(t, trans.HasReadToBottom)
			require.True(t, trans.HasBlock)
			// Victim must be a different (unlocked) way.
			require.NotEqual(t, lockedWay, trans.BlockWayID)
		})

		t.Run("should stall if mshr is full", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
			next.DirPostBuf.Push(0)

			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x200)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x300)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x400)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x500)

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should stall if send to bottom failed", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			readMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x104,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
			next.DirPostBuf.Push(0)

			fillBottomOutgoing(t)

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})
	})

	t.Run("write mshr hit", func(t *testing.T) {
		t.Run("should add to mshr entry and wait for fill", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			// Pre-existing fetcher transaction (index 0) that allocated
			// the MSHR — required so the coalesced write can record it
			// as MSHRFillFetcherIdx.
			fetcherReadMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           fetcherReadMeta,
					ReadAddress:        0x100,
					ReadAccessByteSize: 4,
					ReadPID:            1,
					HasReadToBottom:    true,
				},
			)

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x104,
					WriteData:    []byte{1, 2, 3, 4},
					WritePID:     1,
				},
			)

			// Pre-populate MSHR with the fetcher at index 0.
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			next.MSHRState.Entries[entryIdx].TransactionIndices = []int{0}
			next.DirPostBuf.Push(1)

			madeProgress := d.Tick()

			require.True(t, madeProgress)

			writeToBottomValue, _ := bottomPort.RetrieveOutgoing()

			writeToBottom := writeToBottomValue
			require.Equal(t, uint64(0x104), writeToBottom.Payload.(memprotocol.WriteReq).Address)
			require.Equal(t, []byte{1, 2, 3, 4}, writeToBottom.Payload.(memprotocol.WriteReq).Data)
			require.Equal(t, vm.PID(1), writeToBottom.Payload.(memprotocol.WriteReq).PID)
			entry := next.MSHRState.Entries[entryIdx]
			require.Contains(t, entry.TransactionIndices, 1)
			trans := &next.Transactions[1]
			require.True(t, trans.HasWriteToBottom)
			// The coalesced write must wait for the fetcher's merged-line
			// write to land in storage before completing upstream.
			require.True(t, trans.WaitForMSHRFill)
			require.Equal(t, 0, trans.MSHRFillFetcherIdx)
			require.False(t, trans.MSHRFillDone)
		})
	})

	t.Run("write hit", func(t *testing.T) {
		t.Run("should send to bank", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x104,
					WriteData:    []byte{1, 2, 3, 4},
					WritePID:     1,
				},
			)

			// Set up a valid block
			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()

			wValue, _ := bottomPort.RetrieveOutgoing()

			w := wValue
			require.Equal(t, uint64(0x104), w.Payload.(memprotocol.WriteReq).Address)
			require.Equal(t, []byte{1, 2, 3, 4}, w.Payload.(memprotocol.WriteReq).Data)
			require.Equal(t, vm.PID(1), w.Payload.(memprotocol.WriteReq).PID)

			trans := &next.Transactions[0]
			require.True(t, madeProgress)
			require.True(t, next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked)
			require.True(t, trans.HasWriteToBottom)
			require.Equal(t, bankActionWrite, trans.BankAction)
			require.True(t, trans.HasBlock)
		})

		t.Run("should stall if the block is locked", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x104,
					WriteData:    []byte{1, 2, 3, 4},
					WritePID:     1,
				},
			)

			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked = true

			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should stall if the block is being read", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x104,
					WriteData:    []byte{1, 2, 3, 4},
					WritePID:     1,
				},
			)

			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].ReadCount = 1

			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should stall if bank buf is full", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x104,
					WriteData:    []byte{1, 2, 3, 4},
					WritePID:     1,
				},
			)

			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.Push(0)

			next.BankBufs[0] = queueing.MakeBuffer[int](0)

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should stall if send to bottom failed", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 4 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x104,
					WriteData:    []byte{1, 2, 3, 4},
					WritePID:     1,
				},
			)

			setID := cache.DirectorySetID(0x100, 64, 16)
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.Push(0)

			fillBottomOutgoing(t)

			madeProgress := d.Tick()

			require.False(t, madeProgress)
		})
	})

	t.Run("write miss", func(t *testing.T) {
		t.Run("should send to bottom", func(t *testing.T) {
			setup1(t)

			next := &c.comp.State

			writeMeta := messaging.Msg{
				ID:           c.comp.NewID(),
				TrafficBytes: 64 + 12,
				TrafficClass: "req",
			}
			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x100,
					WriteData:    make([]byte, 64),
					WritePID:     1,
				},
			)
			next.DirPostBuf.Push(0)

			madeProgress := d.Tick()

			wValue, _ := bottomPort.RetrieveOutgoing()

			w := wValue
			require.Equal(t, uint64(0x100), w.Payload.(memprotocol.WriteReq).Address)
			require.Len(t, w.Payload.(memprotocol.WriteReq).Data, 64)
			require.Equal(t, vm.PID(1), w.Payload.(memprotocol.WriteReq).PID)

			trans := &next.Transactions[0]
			require.True(t, madeProgress)
			require.True(t, trans.HasWriteToBottom)
		})
	})
}
