package writeevict

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/mem/cache"
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/akita/v3/sim"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl        *gomock.Controller
		inBuf           *MockBuffer
		dir             *MockDirectory
		mshr            *MockMSHR
		bankBuf         *MockBuffer
		bottomPort      *MockPort
		lowModuleFinder *MockLowModuleFinder
		pipeline        *MockPipeline
		buf             *MockBuffer
		d               *directory
		c               *Cache
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		dir = NewMockDirectory(mockCtrl)
		dir.EXPECT().WayAssociativity().Return(4).AnyTimes()
		mshr = NewMockMSHR(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		lowModuleFinder = NewMockLowModuleFinder(mockCtrl)
		pipeline = NewMockPipeline(mockCtrl)
		buf = NewMockBuffer(mockCtrl)
		c = &Cache{
			log2BlockSize:    6,
			bottomPort:       bottomPort,
			directory:        dir,
			dirBuf:           inBuf,
			numReqPerCycle:   4,
			lowModuleFinder:  lowModuleFinder,
			mshr:             mshr,
			wayAssociativity: 4,
			bankBufs:         []sim.Buffer{bankBuf},
		}
		c.TickingComponent = sim.NewTickingComponent(
			"Cache", nil, 1, c)
		d = &directory{
			cache:    c,
			pipeline: pipeline,
			buf:      buf,
		}

		pipeline.EXPECT().Tick(gomock.Any()).AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transaction", func() {
		pipeline.EXPECT().CanAccept().Return(true)
		inBuf.EXPECT().Peek().Return(nil)
		buf.EXPECT().Peek().Return(nil)

		madeProgress := d.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	Context("read mshr hit", func() {
		var (
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				Build()

			trans = &transaction{
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

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
		})
	})

	Context("read hit", func() {
		var (
			block *cache.Block
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			block = &cache.Block{
				IsValid: true,
			}
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				Build()
			trans = &transaction{
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
				Do(func(t *transaction) {
					Expect(t.block).To(BeIdenticalTo(block))
					Expect(t.bankAction).To(Equal(bankActionReadHit))
				})
			buf.EXPECT().Pop()

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(block.ReadCount).To(Equal(1))
		})

		It("should stall if cannot send to bank", func() {
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			block.IsLocked = true
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			madeProgress := d.Tick(10)
			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("read miss", func() {
		var (
			block     *cache.Block
			read      *mem.ReadReq
			trans     *transaction
			mshrEntry *cache.MSHREntry
		)

		BeforeEach(func() {
			block = &cache.Block{
				IsValid: true,
			}
			mshrEntry = &cache.MSHREntry{}
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				Build()
			trans = &transaction{
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
			lowModuleFinder.EXPECT().Find(uint64(0x100)).Return(nil)
			bottomPort.EXPECT().Send(gomock.Any()).Do(func(read *mem.ReadReq) {
				readToBottom = read
				Expect(read.Address).To(Equal(uint64(0x100)))
				Expect(read.AccessByteSize).To(Equal(uint64(64)))
				Expect(read.PID).To(Equal(vm.PID(1)))
			})
			mshr.EXPECT().IsFull().Return(false)
			mshr.EXPECT().Add(vm.PID(1), uint64(0x100)).Return(mshrEntry)
			buf.EXPECT().Pop()

			madeProgress := d.Tick(10)

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

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is victim block is being read", func() {
			block.ReadCount = 1
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is mshr is full", func() {
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			mshr.EXPECT().IsFull().Return(true)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			lowModuleFinder.EXPECT().Find(uint64(0x100)).Return(nil)
			mshr.EXPECT().IsFull().Return(false)
			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write mshr hit", func() {
		var (
			write     *mem.WriteReq
			trans     *transaction
			mshrEntry *cache.MSHREntry
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x104).
				WithPID(1).
				WithData([]byte{1, 2, 3, 4}).
				Build()
			trans = &transaction{
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
			lowModuleFinder.EXPECT().Find(uint64(0x104))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					writeToBottom = write
					Expect(write.Address).To(Equal(uint64(0x104)))
					Expect(write.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(write.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
			Expect(trans.writeToBottom).To(BeIdenticalTo(writeToBottom))
		})
	})

	Context("write hit", func() {
		var (
			write *mem.WriteReq
			trans *transaction
			block *cache.Block
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x104).
				WithPID(1).
				WithData([]byte{1, 2, 3, 4}).
				Build()
			trans = &transaction{
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
			lowModuleFinder.EXPECT().Find(uint64(0x104))
			bankBuf.EXPECT().CanPush().Return(true)
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					Expect(write.Address).To(Equal(uint64(0x104)))
					Expect(write.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(write.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})

		It("should stall is the block is locked", func() {
			block.IsLocked = true

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is the block is being read", func() {
			block.ReadCount = 1

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is send to bottom failed", func() {
			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
			mshr.EXPECT().Query(vm.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(vm.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(true)
			lowModuleFinder.EXPECT().Find(uint64(0x104))
			bottomPort.EXPECT().Send(gomock.Any()).Return(&sim.SendError{})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write miss", func() {
		var (
			write *mem.WriteReq
			trans *transaction
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x100).
				WithPID(1).
				WithData(make([]byte, 64)).
				Build()
			trans = &transaction{
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
			lowModuleFinder.EXPECT().Find(uint64(0x100))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					Expect(write.Address).To(Equal(uint64(0x100)))
					Expect(write.Data).To(HaveLen(64))
					Expect(write.PID).To(Equal(vm.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})
	})

})
