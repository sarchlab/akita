package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("Directory", func() {
	var (
		bottomPort messaging.Port
		d          *directory
		c          *pipelineMW
	)

	// fillBottomOutgoing pre-fills bottomPort's single outgoing slot so the
	// next CanSend returns false, simulating a busy port.
	fillBottomOutgoing := func() {
		dummy := memprotocol.ReadReq{}
		dummy.ID = timing.GetIDGenerator().Generate()
		dummy.Src = bottomPort.AsRemote()
		dummy.Dst = messaging.RemotePort("DRAM")
		dummy.TrafficClass = "req"
		Expect(bottomPort.CanSend()).To(BeTrue())
		bottomPort.Send(dummy)
	}

	BeforeEach(func() {
		c = &pipelineMW{}

		initialState := State{
			DirBuf: queueing.NewBuffer[int]("Cache.DirBuf", 4),
			BankBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.BankBuf0", 4),
			},
			DirPipeline: queueing.NewPipeline[int](4, 2),
			DirPostBuf:  queueing.NewBuffer[int]("Cache.DirPostBuf", 4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.NewPipeline[int](4, 10),
			},
			BankPostBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.BankPostBuf0", 4),
			},
		}

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		c.comp = modeling.NewBuilder[Spec, State, Resources]().
			WithEngine(timing.NewSerialEngine()).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				Log2BlockSize:     6,
				NumReqPerCycle:    4,
				WayAssociativity:  4,
				NumMSHREntry:      4,
				NumSets:           16,
				NumBanks:          1,
				AddressMapperType: "single",
				RemotePortNames:   []string{"DRAM"},
				WritePolicyType:   "write-around",
			}).
			Build("Cache")

		// bottomPort is a real, single-slot port owned by the component.
		// Success cases read the sent request back via RetrieveOutgoing;
		// failure cases pre-fill the slot. The directory stage resolves it
		// lazily via GetPortByName("Bottom"), so it is declared and assigned a
		// real port.
		bottomPort = messaging.NewPort(c.comp, 1, 1, "Cache.Bottom")
		(&noopConn{}).PlugIn(bottomPort)
		c.comp.DeclarePort("Bottom")
		c.comp.AssignPort("Bottom", bottomPort)

		c.comp.State = initialState

		d = &directory{
			cache: c,
		}
	})

	It("should do nothing if no transaction", func() {
		madeProgress := d.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	Context("read mshr hit", func() {
		It("Should add to mshr entry", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			entry := next.MSHRState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(0))
		})
	})

	Context("read hit", func() {
		It("should send transaction to bank", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()

			trans := &next.Transactions[0]
			Expect(madeProgress).To(BeTrue())
			Expect(trans.HasBlock).To(BeTrue())
			Expect(trans.BlockSetID).To(Equal(setID))
			Expect(trans.BlockWayID).To(Equal(wayID))
			Expect(trans.BankAction).To(Equal(bankActionReadHit))
			Expect(next.DirectoryState.Sets[setID].Blocks[wayID].ReadCount).To(Equal(1))
			// Bank buf should have the trans index
			Expect(next.BankBufs[0].Size()).To(Equal(1))
		})

		It("should stall if cannot send to bank", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.PushTyped(0)

			// Fill up bank buffer
			next.BankBufs[0] = queueing.NewBuffer[int]("Cache.BankBuf0", 0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked = true

			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()
			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("read miss", func() {
		It("should send request to bottom", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())

			readToBottom := bottomPort.RetrieveOutgoing().(memprotocol.ReadReq)
			Expect(readToBottom.Address).To(Equal(uint64(0x100)))
			Expect(readToBottom.AccessByteSize).To(Equal(uint64(64)))
			Expect(readToBottom.PID).To(Equal(vm.PID(1)))
			// Check MSHR entry was created
			entryIdx, found := cache.MSHRQuery(&next.MSHRState, vm.PID(1), 0x100)
			Expect(found).To(BeTrue())
			entry := next.MSHRState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(0))
			Expect(entry.HasBlock).To(BeTrue())
			Expect(entry.HasReadReq).To(BeTrue())

			// Check victim block was set up
			victimSetID := entry.BlockSetID
			victimWayID := entry.BlockWayID
			block := next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
			Expect(block.Tag).To(Equal(uint64(0x100)))
			Expect(block.IsLocked).To(BeTrue())
			Expect(block.IsValid).To(BeTrue())
			trans := &next.Transactions[0]
			Expect(trans.HasReadToBottom).To(BeTrue())
			Expect(trans.HasBlock).To(BeTrue())
		})

		It("should stall if every way in the set is locked", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			setID := 4 // (0x100 / 64) % 16 = 4
			for w := range next.DirectoryState.Sets[setID].Blocks {
				next.DirectoryState.Sets[setID].Blocks[w].IsLocked = true
			}

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if every way in the set is being read", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			setID := 4
			for w := range next.DirectoryState.Sets[setID].Blocks {
				next.DirectoryState.Sets[setID].Blocks[w].ReadCount = 1
			}

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should skip the LRU victim if it is locked and try another way", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			setID := 4
			// Lock only the LRU-most way; other ways should still be picked.
			lockedWay := next.DirectoryState.Sets[setID].LRUOrder[0]
			next.DirectoryState.Sets[setID].Blocks[lockedWay].IsLocked = true

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			trans := &next.Transactions[0]
			Expect(trans.HasReadToBottom).To(BeTrue())
			Expect(trans.HasBlock).To(BeTrue())
			// Victim must be a different (unlocked) way.
			Expect(trans.BlockWayID).NotTo(Equal(lockedWay))
		})

		It("should stall if mshr is full", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x200)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x300)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x400)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x500)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			next := &c.comp.State

			readMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			fillBottomOutgoing()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write mshr hit", func() {
		It("should add to mshr entry and wait for fill", func() {
			next := &c.comp.State

			// Pre-existing fetcher transaction (index 0) that allocated
			// the MSHR — required so the coalesced write can record it
			// as MSHRFillFetcherIdx.
			fetcherReadMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(1)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())

			writeToBottom := bottomPort.RetrieveOutgoing().(memprotocol.WriteReq)
			Expect(writeToBottom.Address).To(Equal(uint64(0x104)))
			Expect(writeToBottom.Data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(writeToBottom.PID).To(Equal(vm.PID(1)))
			entry := next.MSHRState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(1))
			trans := &next.Transactions[1]
			Expect(trans.HasWriteToBottom).To(BeTrue())
			// The coalesced write must wait for the fetcher's merged-line
			// write to land in storage before completing upstream.
			Expect(trans.WaitForMSHRFill).To(BeTrue())
			Expect(trans.MSHRFillFetcherIdx).To(Equal(0))
			Expect(trans.MSHRFillDone).To(BeFalse())
		})
	})

	Context("write hit", func() {
		It("should send to bank", func() {
			next := &c.comp.State

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()

			w := bottomPort.RetrieveOutgoing().(memprotocol.WriteReq)
			Expect(w.Address).To(Equal(uint64(0x104)))
			Expect(w.Data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(w.PID).To(Equal(vm.PID(1)))

			trans := &next.Transactions[0]
			Expect(madeProgress).To(BeTrue())
			Expect(next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked).To(BeTrue())
			Expect(trans.HasWriteToBottom).To(BeTrue())
			Expect(trans.BankAction).To(Equal(bankActionWrite))
			Expect(trans.HasBlock).To(BeTrue())
		})

		It("should stall if the block is locked", func() {
			next := &c.comp.State

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked = true

			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if the block is being read", func() {
			next := &c.comp.State

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].ReadCount = 1

			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			next := &c.comp.State

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.PushTyped(0)

			next.BankBufs[0] = queueing.NewBuffer[int]("Cache.BankBuf0", 0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			next := &c.comp.State

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			next.DirPostBuf.PushTyped(0)

			fillBottomOutgoing()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write miss", func() {
		It("should send to bottom", func() {
			next := &c.comp.State

			writeMeta := messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
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
			next.DirPostBuf.PushTyped(0)

			madeProgress := d.Tick()

			w := bottomPort.RetrieveOutgoing().(memprotocol.WriteReq)
			Expect(w.Address).To(Equal(uint64(0x100)))
			Expect(w.Data).To(HaveLen(64))
			Expect(w.PID).To(Equal(vm.PID(1)))

			trans := &next.Transactions[0]
			Expect(madeProgress).To(BeTrue())
			Expect(trans.HasWriteToBottom).To(BeTrue())
		})
	})

})
