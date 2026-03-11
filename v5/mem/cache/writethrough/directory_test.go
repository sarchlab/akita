package writethrough

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl   *gomock.Controller
		bottomPort *MockPort
		d          *directory
		c          *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		c = &middleware{
			bottomPort: bottomPort,
		}

		initialState := State{
			BankBufIndices:             []bankBufState{{Indices: nil}},
			BankPipelineStages:         []bankPipelineState{{Stages: nil}},
			BankPostPipelineBufIndices: []bankPostBufState{{Indices: nil}},
		}

		// Initialize directoryState before SetState so both buffers match
		cache.DirectoryReset(&initialState.DirectoryState, 16, 4, 64)

		c.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumMSHREntry:     4,
				NumSets:          16,
				NumBanks:         1,
				AddressMapperType: "single",
				RemotePortNames:   []string{""},
			}).
			Build("Cache")

		c.comp.SetState(initialState)

		next := c.comp.GetNextState()

		// Create adapters
		c.dirBufAdapter = &stateTransBuffer{
			name:       "Cache.DirBuf",
			readItems:  &next.DirBufIndices,
			writeItems: &next.DirBufIndices,
			capacity:   4,
			mw:         c,
		}
		c.bankBufAdapters = []*stateTransBuffer{
			{
				name:       "Cache.BankBuf0",
				readItems:  &next.BankBufIndices[0].Indices,
				writeItems: &next.BankBufIndices[0].Indices,
				capacity:   4,
				mw:         c,
			},
		}
		c.dirPostBufAdapter = &stateDirPostBufAdapter{
			name:       "Cache.DirPostBuf",
			readItems:  &next.DirPostPipelineBufIndices,
			writeItems: &next.DirPostPipelineBufIndices,
			capacity:   4,
			mw:         c,
		}

		d = &directory{
			cache: c,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transaction", func() {
		c.syncForTest()
		madeProgress := d.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	Context("read mshr hit", func() {
		var (
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x104
			read.PID = 1
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "req"

			trans = &transactionState{
				read: read,
			}
		})

		It("Should add to mshr entry", func() {
			next := c.comp.GetNextState()

			// Pre-populate MSHR with an entry
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			// The trans is a postCoalesceTransaction, so register it
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)

			// Put trans in post-pipeline buffer
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			entry := next.MSHRState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(0))
		})
	})

	Context("read hit", func() {
		var (
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x104
			read.PID = 1
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "req"
			trans = &transactionState{
				read: read,
			}
		})

		It("should send transaction to bank", func() {
			next := c.comp.GetNextState()

			// Set up a valid block in directory at the right set for address 0x100
			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.hasBlock).To(BeTrue())
			Expect(trans.blockSetID).To(Equal(setID))
			Expect(trans.blockWayID).To(Equal(wayID))
			Expect(trans.bankAction).To(Equal(bankActionReadHit))
			Expect(next.DirectoryState.Sets[setID].Blocks[wayID].ReadCount).To(Equal(1))
			// Bank buf should have the trans index
			Expect(next.BankBufIndices[0].Indices).To(HaveLen(1))
		})

		It("should stall if cannot send to bank", func() {
			next := c.comp.GetNextState()

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			// Fill up bank buffer
			c.bankBufAdapters[0].capacity = 0

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			next := c.comp.GetNextState()

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked = true

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			c.syncForTest()

			madeProgress := d.Tick()
			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("read miss", func() {
		var (
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x104
			read.PID = 1
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "req"
			trans = &transactionState{
				read: read,
			}
		})

		It("should send request to bottom", func() {
			next := c.comp.GetNextState()
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			var readToBottom *mem.ReadReq
			bottomPort.EXPECT().Send(gomock.Any()).Do(func(msg sim.Msg) {
				readToBottom = msg.(*mem.ReadReq)
				Expect(readToBottom.Address).To(Equal(uint64(0x100)))
				Expect(readToBottom.AccessByteSize).To(Equal(uint64(64)))
				Expect(readToBottom.PID).To(Equal(vm.PID(1)))
			})

			c.syncForTest()

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
			Expect(trans.readToBottom).To(BeIdenticalTo(readToBottom))
			Expect(trans.hasBlock).To(BeTrue())
		})

		It("should stall if victim block is locked", func() {
			next := c.comp.GetNextState()
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			setID := 4 // (0x100 / 64) % 16 = 4
			next.DirectoryState.Sets[setID].Blocks[next.DirectoryState.Sets[setID].LRUOrder[0]].IsLocked = true

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if victim block is being read", func() {
			next := c.comp.GetNextState()
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			setID := 4
			next.DirectoryState.Sets[setID].Blocks[next.DirectoryState.Sets[setID].LRUOrder[0]].ReadCount = 1

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if mshr is full", func() {
			next := c.comp.GetNextState()
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x200)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x300)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x400)
			cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), 0x500)

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			next := c.comp.GetNextState()
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write mshr hit", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
		)

		BeforeEach(func() {
			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x104
			write.PID = 1
			write.Data = []byte{1, 2, 3, 4}
			write.TrafficBytes = 4 + 12
			write.TrafficClass = "req"
			trans = &transactionState{
				write: write,
			}
		})

		It("should add to mshr entry", func() {
			next := c.comp.GetNextState()

			var writeToBottom *mem.WriteReq

			// Pre-populate MSHR
			entryIdx := cache.MSHRAdd(&next.MSHRState, 4, vm.PID(1), uint64(0x100))
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					writeToBottom = msg.(*mem.WriteReq)
					Expect(writeToBottom.Address).To(Equal(uint64(0x104)))
					Expect(writeToBottom.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(writeToBottom.PID).To(Equal(vm.PID(1)))
				})

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			entry := next.MSHRState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(0))
			Expect(trans.writeToBottom).To(BeIdenticalTo(writeToBottom))
		})
	})

	Context("write hit", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
		)

		BeforeEach(func() {
			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x104
			write.PID = 1
			write.Data = []byte{1, 2, 3, 4}
			write.TrafficBytes = 4 + 12
			write.TrafficClass = "req"
			trans = &transactionState{
				write: write,
			}
		})

		It("should send to bank", func() {
			next := c.comp.GetNextState()

			// Set up a valid block
			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					w := msg.(*mem.WriteReq)
					Expect(w.Address).To(Equal(uint64(0x104)))
					Expect(w.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(w.PID).To(Equal(vm.PID(1)))
				})

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
			Expect(trans.bankAction).To(Equal(bankActionWrite))
			Expect(trans.hasBlock).To(BeTrue())
		})

		It("should stall if the block is locked", func() {
			next := c.comp.GetNextState()

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].IsLocked = true

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if the block is being read", func() {
			next := c.comp.GetNextState()

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1
			next.DirectoryState.Sets[setID].Blocks[wayID].ReadCount = 1

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			next := c.comp.GetNextState()

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			c.bankBufAdapters[0].capacity = 0

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			next := c.comp.GetNextState()

			setID := 4
			wayID := 0
			next.DirectoryState.Sets[setID].Blocks[wayID].IsValid = true
			next.DirectoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			next.DirectoryState.Sets[setID].Blocks[wayID].PID = 1

			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write miss", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
		)

		BeforeEach(func() {
			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x100
			write.PID = 1
			write.Data = make([]byte, 64)
			write.TrafficBytes = 64 + 12
			write.TrafficClass = "req"
			trans = &transactionState{
				write: write,
			}
		})

		It("should send to bottom", func() {
			next := c.comp.GetNextState()
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)
			next.DirPostPipelineBufIndices = append(next.DirPostPipelineBufIndices, 0)

			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					w := msg.(*mem.WriteReq)
					Expect(w.Address).To(Equal(uint64(0x100)))
					Expect(w.Data).To(HaveLen(64))
					Expect(w.PID).To(Equal(vm.PID(1)))
				})

			c.syncForTest()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})
	})

})
