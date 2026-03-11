package writearound

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl            *gomock.Controller
		inBuf               *MockBuffer
		bankBuf             *MockBuffer
		bottomPort          *MockPort
		addressToPortMapper *MockAddressToPortMapper
		pipeline            *MockPipeline
		buf                 *MockBuffer
		d                   *directory
		c                   *middleware
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		pipeline = NewMockPipeline(mockCtrl)
		buf = NewMockBuffer(mockCtrl)
		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)
		c = &middleware{
			bottomPort:          bottomPort,
			dirBuf:              inBuf,
			addressToPortMapper: addressToPortMapper,
			bankBufs:            []queueing.Buffer{bankBuf},
		}
		c.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
				NumMSHREntry:     4,
				NumSets:          16,
			}).
			Build("Cache")

		// Initialize directoryState with 16 sets, 4 ways, blockSize=64
		cache.DirectoryReset(&c.directoryState, 16, 4, 64)

		d = &directory{
			cache:    c,
			pipeline: pipeline,
			buf:      buf,
		}

		pipeline.EXPECT().Tick().AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transaction", func() {
		pipeline.EXPECT().CanAccept().Return(true)
		inBuf.EXPECT().Peek().Return(nil)
		buf.EXPECT().Peek().Return(nil)

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

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
		})

		It("Should add to mshr entry", func() {
			// Pre-populate MSHR with an entry
			entryIdx := cache.MSHRAdd(&c.mshrState, 4, vm.PID(1), uint64(0x100))
			// The trans is a postCoalesceTransaction, so register it
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)

			buf.EXPECT().Pop()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			entry := c.mshrState.Entries[entryIdx]
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

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
		})

		It("should send transaction to bank", func() {
			// Set up a valid block in directory at the right set for address 0x100
			// setID = (0x100 / 64) % 16 = 4 % 16 = 4
			setID := 4
			wayID := 0
			c.directoryState.Sets[setID].Blocks[wayID].IsValid = true
			c.directoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			c.directoryState.Sets[setID].Blocks[wayID].PID = 1

			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(t *transactionState) {
					Expect(t.hasBlock).To(BeTrue())
					Expect(t.blockSetID).To(Equal(setID))
					Expect(t.blockWayID).To(Equal(wayID))
					Expect(t.bankAction).To(Equal(bankActionReadHit))
				})
			buf.EXPECT().Pop()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(c.directoryState.Sets[setID].Blocks[wayID].ReadCount).To(Equal(1))
		})

		It("should stall if cannot send to bank", func() {
			setID := 4
			wayID := 0
			c.directoryState.Sets[setID].Blocks[wayID].IsValid = true
			c.directoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			c.directoryState.Sets[setID].Blocks[wayID].PID = 1

			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			setID := 4
			wayID := 0
			c.directoryState.Sets[setID].Blocks[wayID].IsValid = true
			c.directoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			c.directoryState.Sets[setID].Blocks[wayID].PID = 1
			c.directoryState.Sets[setID].Blocks[wayID].IsLocked = true

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

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
		})

		It("should send request to bottom", func() {
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)

			var readToBottom *mem.ReadReq
			addressToPortMapper.EXPECT().
				Find(uint64(0x100)).
				Return(sim.RemotePort(""))
			bottomPort.EXPECT().Send(gomock.Any()).Do(func(msg sim.Msg) {
				readToBottom = msg.(*mem.ReadReq)
				Expect(readToBottom.Address).To(Equal(uint64(0x100)))
				Expect(readToBottom.AccessByteSize).To(Equal(uint64(64)))
				Expect(readToBottom.PID).To(Equal(vm.PID(1)))
			})
			buf.EXPECT().Pop()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			// Check MSHR entry was created
			entryIdx, found := cache.MSHRQuery(&c.mshrState, vm.PID(1), 0x100)
			Expect(found).To(BeTrue())
			entry := c.mshrState.Entries[entryIdx]
			Expect(entry.TransactionIndices).To(ContainElement(0))
			Expect(entry.HasBlock).To(BeTrue())
			Expect(entry.HasReadReq).To(BeTrue())

			// Check victim block was set up
			victimSetID := entry.BlockSetID
			victimWayID := entry.BlockWayID
			block := c.directoryState.Sets[victimSetID].Blocks[victimWayID]
			Expect(block.Tag).To(Equal(uint64(0x100)))
			Expect(block.IsLocked).To(BeTrue())
			Expect(block.IsValid).To(BeTrue())
			Expect(trans.readToBottom).To(BeIdenticalTo(readToBottom))
			Expect(trans.hasBlock).To(BeTrue())
		})

		It("should stall if victim block is locked", func() {
			// Lock all victim candidates (LRU[0] for each way)
			setID := 4 // (0x100 / 64) % 16 = 4
			c.directoryState.Sets[setID].Blocks[c.directoryState.Sets[setID].LRUOrder[0]].IsLocked = true

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if victim block is being read", func() {
			setID := 4
			c.directoryState.Sets[setID].Blocks[c.directoryState.Sets[setID].LRUOrder[0]].ReadCount = 1

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if mshr is full", func() {
			// Fill up MSHR
			cache.MSHRAdd(&c.mshrState, 4, vm.PID(1), 0x200)
			cache.MSHRAdd(&c.mshrState, 4, vm.PID(1), 0x300)
			cache.MSHRAdd(&c.mshrState, 4, vm.PID(1), 0x400)
			cache.MSHRAdd(&c.mshrState, 4, vm.PID(1), 0x500)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)

			addressToPortMapper.EXPECT().
				Find(uint64(0x100)).
				Return(sim.RemotePort(""))
			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

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
			var writeToBottom *mem.WriteReq

			// Pre-populate MSHR
			entryIdx := cache.MSHRAdd(&c.mshrState, 4, vm.PID(1), uint64(0x100))
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, trans)

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			buf.EXPECT().Pop()
			addressToPortMapper.EXPECT().Find(uint64(0x104))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					writeToBottom = msg.(*mem.WriteReq)
					Expect(writeToBottom.Address).To(Equal(uint64(0x104)))
					Expect(writeToBottom.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(writeToBottom.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			entry := c.mshrState.Entries[entryIdx]
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

			// Set up a valid block
			setID := 4 // (0x100 / 64) % 16 = 4
			wayID := 0
			c.directoryState.Sets[setID].Blocks[wayID].IsValid = true
			c.directoryState.Sets[setID].Blocks[wayID].Tag = 0x100
			c.directoryState.Sets[setID].Blocks[wayID].PID = 1
		})

		It("should send to bank", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			buf.EXPECT().Pop()
			addressToPortMapper.EXPECT().Find(uint64(0x104))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transactionState) {
					Expect(trans.bankAction).To(Equal(bankActionWrite))
					Expect(trans.hasBlock).To(BeTrue())
				})
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					w := msg.(*mem.WriteReq)
					Expect(w.Address).To(Equal(uint64(0x104)))
					Expect(w.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(w.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			setID := 4
			wayID := 0
			Expect(c.directoryState.Sets[setID].Blocks[wayID].IsLocked).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})

		It("should stall if the block is locked", func() {
			setID := 4
			wayID := 0
			c.directoryState.Sets[setID].Blocks[wayID].IsLocked = true

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if the block is being read", func() {
			setID := 4
			wayID := 0
			c.directoryState.Sets[setID].Blocks[wayID].ReadCount = 1

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			bankBuf.EXPECT().CanPush().Return(true)
			addressToPortMapper.EXPECT().Find(uint64(0x104))
			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

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
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			buf.EXPECT().Pop()
			addressToPortMapper.EXPECT().Find(uint64(0x100))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					w := msg.(*mem.WriteReq)
					Expect(w.Address).To(Equal(uint64(0x100)))
					Expect(w.Data).To(HaveLen(64))
					Expect(w.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})
	})

})
