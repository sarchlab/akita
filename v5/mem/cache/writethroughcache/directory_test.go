package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl   *gomock.Controller
		bottomPort *MockPort
		d          *directory
		c          *pipelineMW
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		c = &pipelineMW{
			bottomPort: bottomPort,
		}

		initialState := State{
			DirBuf: queueing.Buffer[int]{
				BufferName: "Cache.DirBuf",
				Cap:        4,
			},
			BankBufs: []queueing.Buffer[int]{
				{BufferName: "Cache.BankBuf0", Cap: 4},
			},
			DirPipeline: queueing.Pipeline[int]{
				Width: 4, NumStages: 2,
			},
			DirPostBuf: queueing.Buffer[int]{
				BufferName: "Cache.DirPostBuf",
				Cap:        4,
			},
			BankPipelines: []queueing.Pipeline[int]{
				{Width: 4, NumStages: 10},
			},
			BankPostBufs: []queueing.Buffer[int]{
				{BufferName: "Cache.BankPostBuf0", Cap: 4},
			},
		}

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		c.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:     6,
				NumReqPerCycle:    4,
				WayAssociativity:  4,
				NumMSHREntry:      4,
				NumSets:           16,
				NumBanks:          1,
				AddressMapperType: "single",
				RemotePortNames:   []string{"DRAM"},
			}).
			Build("Cache")

		c.comp.SetState(initialState)

		c.writePolicy = &WritearoundPolicy{}

		d = &directory{
			cache:       c,
			writePolicy: &WritearoundPolicy{},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transaction", func() {
		madeProgress := d.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	Context("read mshr hit", func() {
		It("Should add to mshr entry", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			entry := next.MSHRState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(0))
		})
	})

	Context("read hit", func() {
		It("should send transaction to bank", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			madeProgress := d.Tick()

			trans := &next.Transactions[0]
			Expect(madeProgress).To(BeTrue())
			Expect(trans.HasBlock).To(BeTrue())
			Expect(trans.BlockSetID).To(Equal(setID))
			Expect(trans.BlockWayID).To(Equal(wayID))
			Expect(trans.BankAction).To(Equal(bankActionReadHit))
			Expect(next.DirectoryState.Sets[setID].Blocks[wayID].ReadCount).To(Equal(1))
			// Bank buf should have the trans index
			Expect(next.BankBufs[0].Elements).To(HaveLen(1))
		})

		It("should stall if cannot send to bank", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			// Fill up bank buffer
			next.BankBufs[0].Cap = 0

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			madeProgress := d.Tick()
			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("read miss", func() {
		It("should send request to bottom", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			bottomPort.EXPECT().Send(gomock.Any()).Do(func(msg sim.Msg) {
				readToBottom := msg.(*mem.ReadReq)
				Expect(readToBottom.Address).To(Equal(uint64(0x100)))
				Expect(readToBottom.AccessByteSize).To(Equal(uint64(64)))
				Expect(readToBottom.PID).To(Equal(vm.PID(1)))
			})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
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

		It("should stall if victim block is locked", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			setID := 4 // (0x100 / 64) % 16 = 4
			next.DirectoryState.Sets[setID].Blocks[next.DirectoryState.Sets[setID].LRUOrder[0]].IsLocked = true

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if victim block is being read", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			setID := 4
			next.DirectoryState.Sets[setID].Blocks[next.DirectoryState.Sets[setID].LRUOrder[0]].ReadCount = 1

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if mshr is full", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x200)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x300)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x400)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x500)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			next := c.comp.GetNextState()

			readMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write mshr hit", func() {
		It("should add to mshr entry", func() {
			next := c.comp.GetNextState()

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			// Pre-populate MSHR
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					writeToBottom := msg.(*mem.WriteReq)
					Expect(writeToBottom.Address).To(Equal(uint64(0x104)))
					Expect(writeToBottom.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(writeToBottom.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			entry := next.MSHRState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(0))
			trans := &next.Transactions[0]
			Expect(trans.HasWriteToBottom).To(BeTrue())
		})
	})

	Context("write hit", func() {
		It("should send to bank", func() {
			next := c.comp.GetNextState()

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					w := msg.(*mem.WriteReq)
					Expect(w.Address).To(Equal(uint64(0x104)))
					Expect(w.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(w.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick()

			trans := &next.Transactions[0]
			Expect(madeProgress).To(BeTrue())
			Expect(next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked).To(BeTrue())
			Expect(trans.HasWriteToBottom).To(BeTrue())
			Expect(trans.BankAction).To(Equal(bankActionWrite))
			Expect(trans.HasBlock).To(BeTrue())
		})

		It("should stall if the block is locked", func() {
			next := c.comp.GetNextState()

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if the block is being read", func() {
			next := c.comp.GetNextState()

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			next := c.comp.GetNextState()

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			next.BankBufs[0].Cap = 0

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			next := c.comp.GetNextState()

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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

			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write miss", func() {
		It("should send to bottom", func() {
			next := c.comp.GetNextState()

			writeMeta := sim.MsgMeta{
				ID:           sim.GetIDGenerator().Generate(),
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
			next.DirPostBuf.Elements = append(next.DirPostBuf.Elements, 0)

			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					w := msg.(*mem.WriteReq)
					Expect(w.Address).To(Equal(uint64(0x100)))
					Expect(w.Data).To(HaveLen(64))
					Expect(w.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick()

			trans := &next.Transactions[0]
			Expect(madeProgress).To(BeTrue())
			Expect(trans.HasWriteToBottom).To(BeTrue())
		})
	})

})
