package writearound

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl            *gomock.Controller
		inBuf               *MockBuffer
		dir                 *MockDirectory
		mshr                *MockMSHR
		bankBuf             *MockBuffer
		bottomPort          *MockPort
		addressToPortMapper *MockAddressToPortMapper
		pipeline            *MockPipeline
		buf                 *MockBuffer
		d                   *directory
		c                   *Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		dir = NewMockDirectory(mockCtrl)
		dir.EXPECT().WayAssociativity().Return(4).AnyTimes()
		mshr = NewMockMSHR(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)

		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		pipeline = NewMockPipeline(mockCtrl)
		buf = NewMockBuffer(mockCtrl)
		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)
		c = &Comp{
			bottomPort:          bottomPort,
			directory:           dir,
			dirBuf:              inBuf,
			addressToPortMapper: addressToPortMapper,
			mshr:                mshr,
			bankBufs:            []queueing.Buffer{bankBuf},
		}
		c.Component = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:    6,
				NumReqPerCycle:   4,
				WayAssociativity: 4,
			}).
			Build("Cache")
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
			mshrEntry := &cache.MSHREntry{}
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(mshrEntry)
			buf.EXPECT().Pop()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
		})
	})

	Context("read hit", func() {
		var (
			block *cache.Block
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			block = &cache.Block{
				IsValid: true,
			}
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
			mshr.EXPECT().Query(vm.PID(1), gomock.Any()).Return(nil)
		})

		It("should send transaction to bank", func() {
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(t *transactionState) {
					Expect(t.block).To(BeIdenticalTo(block))
					Expect(t.bankAction).To(Equal(bankActionReadHit))
				})
			buf.EXPECT().Pop()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(block.ReadCount).To(Equal(1))
		})

		It("should stall if cannot send to bank", func() {
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			block.IsLocked = true
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			madeProgress := d.Tick()
			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("read miss", func() {
		var (
			block     *cache.Block
			read      *mem.ReadReq
			trans     *transactionState
			mshrEntry *cache.MSHREntry
		)

		BeforeEach(func() {
			block = &cache.Block{
				IsValid: true,
			}
			mshrEntry = &cache.MSHREntry{}
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
			mshr.EXPECT().Query(vm.PID(1), gomock.Any()).Return(nil)
		})

		It("should send request to bottom", func() {
			var readToBottom *mem.ReadReq
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			addressToPortMapper.EXPECT().
				Find(uint64(0x100)).
				Return(sim.RemotePort(""))
			bottomPort.EXPECT().Send(gomock.Any()).Do(func(msg sim.Msg) {
				readToBottom = msg.(*mem.ReadReq)
				Expect(readToBottom.Address).To(Equal(uint64(0x100)))
				Expect(readToBottom.AccessByteSize).To(Equal(uint64(64)))
				Expect(readToBottom.PID).To(Equal(vm.PID(1)))
			})
			mshr.EXPECT().IsFull().Return(false)
			mshr.EXPECT().Add(vm.PID(1), uint64(0x100)).Return(mshrEntry)
			buf.EXPECT().Pop()

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
			Expect(mshrEntry.Block).To(BeIdenticalTo(block))
			Expect(mshrEntry.ReadReq).To(BeIdenticalTo(readToBottom))
			Expect(block.Tag).To(Equal(uint64(0x100)))
			Expect(block.IsLocked).To(BeTrue())
			Expect(block.IsValid).To(BeTrue())
			Expect(trans.readToBottom).To(BeIdenticalTo(readToBottom))
			Expect(trans.block).To(BeIdenticalTo(block))
		})

		It("should stall is victim block is locked", func() {
			block.IsLocked = true
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is victim block is being read", func() {
			block.ReadCount = 1
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is mshr is full", func() {
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			mshr.EXPECT().IsFull().Return(true)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			addressToPortMapper.EXPECT().
				Find(uint64(0x100)).
				Return(sim.RemotePort(""))
			mshr.EXPECT().IsFull().Return(false)
			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write mshr hit", func() {
		var (
			write     *mem.WriteReq
			trans     *transactionState
			mshrEntry *cache.MSHREntry
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
			mshrEntry = &cache.MSHREntry{}
		})

		It("should add to mshr entry", func() {
			var writeToBottom *mem.WriteReq

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			buf.EXPECT().Pop()
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(mshrEntry)
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
			Expect(mshrEntry.Requests).To(ContainElement(trans))
			Expect(trans.writeToBottom).To(BeIdenticalTo(writeToBottom))
		})
	})

	Context("write hit", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
			block *cache.Block
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
			block = &cache.Block{IsValid: true}
		})

		It("should send to bank", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			buf.EXPECT().Pop()
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			addressToPortMapper.EXPECT().Find(uint64(0x104))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transactionState) {
					Expect(trans.bankAction).To(Equal(bankActionWrite))
					Expect(trans.block).To(BeIdenticalTo(block))
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
			Expect(block.IsLocked).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})

		It("should stall is the block is locked", func() {
			block.IsLocked = true

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is the block is being read", func() {
			block.ReadCount = 1

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is send to bottom failed", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
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
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
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
