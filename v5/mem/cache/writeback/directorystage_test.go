package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("DirectoryStage", func() {

	var (
		mockCtrl            *gomock.Controller
		ds                  *directoryStage
		m                   *middleware
		dirBuf              *MockBuffer
		pipeline            *MockPipeline
		buf                 *MockBuffer
		bankBuf             *MockBuffer
		writeBufferBuffer   *MockBuffer
		addressToPortMapper *MockAddressToPortMapper
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		dirBuf = NewMockBuffer(mockCtrl)
		writeBufferBuffer = NewMockBuffer(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)

		comp := MakeBuilder().
			WithEngine(sim.NewSerialEngine()).
			WithAddressToPortMapper(addressToPortMapper).
			WithTopPort(sim.NewPort(nil, 2, 2, "Cache.ToTop")).
			WithBottomPort(sim.NewPort(nil, 2, 2, "Cache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 2, 2, "Cache.ControlPort")).
			Build("Cache")
		m = comp.Middlewares()[0].(*middleware)

		m.dirStageBuffer = dirBuf
		m.numReqPerCycle = 4
		m.writeBufferBuffer = writeBufferBuffer
		m.dirToBankBuffers = []queueing.Buffer{bankBuf}
		m.addressToPortMapper = addressToPortMapper

		// Initialize directory state
		cache.DirectoryReset(&m.directoryState, m.numSets, m.wayAssociativity, m.blockSize)

		pipeline = NewMockPipeline(mockCtrl)
		buf = NewMockBuffer(mockCtrl)
		ds = &directoryStage{
			cache:    m,
			pipeline: pipeline,
			buf:      buf,
		}

		pipeline.EXPECT().Tick().AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should return if no transaction", func() {
		pipeline.EXPECT().CanAccept().Return(true)
		dirBuf.EXPECT().Peek().Return(nil)
		buf.EXPECT().Peek().Return(nil)

		ret := ds.Tick()

		Expect(ret).To(BeFalse())
	})

	Context("read", func() {
		var (
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x100
			read.PID = 1
			read.AccessByteSize = 64
			read.TrafficBytes = 12
			read.TrafficClass = "mem.ReadReq"
			trans = &transactionState{
				read: read,
			}
			m.inFlightTransactions = []*transactionState{trans}

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
		})

		Context("mshr hit", func() {
			BeforeEach(func() {
				// Add MSHR entry for PID 1, address 0x100
				cache.MSHRAdd(&m.mshrState, m.numMSHREntry, vm.PID(1), 0x100)
			})

			It("should add to MSHR", func() {
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(m.mshrState.Entries[0].TransactionIndices).To(HaveLen(1))
			})
		})

		Context("hit", func() {
			BeforeEach(func() {
				// Set up a block for PID 1, address 0x100
				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				block := &m.directoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			})

			It("should stall if bank is busy", func() {
				bankBuf.EXPECT().CanPush().Return(false)

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})

			It("should stall if block is locked", func() {
				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				m.directoryState.Sets[setID].Blocks[0].IsLocked = true

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})

			It("should pass transaction to bank", func() {
				bankBuf.EXPECT().CanPush().Return(true)
				bankBuf.EXPECT().Push(gomock.Any()).
					Do(func(t *transactionState) {
						Expect(t.read).To(BeIdenticalTo(read))
						Expect(t.hasBlock).To(BeTrue())
					})
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				block := &m.directoryState.Sets[setID].Blocks[0]
				Expect(block.ReadCount).To(Equal(1))
				Expect(trans.action).To(Equal(bankReadHit))
			})
		})

		Context("miss, mshr miss, mshr full", func() {
			It("should stall", func() {
				// Fill up MSHR
				for i := 0; i < m.numMSHREntry; i++ {
					cache.MSHRAdd(&m.mshrState, m.numMSHREntry, vm.PID(100), uint64(i*0x1000))
				}

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})
		})

		Context("miss, mshr miss, no need to evict", func() {
			It("should stall if bank buffer is full", func() {
				bankBuf.EXPECT().CanPush().Return(false)

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})

			It("should create mshr entry and fetch", func() {
				bankBuf.EXPECT().CanPush().Return(true)
				bankBuf.EXPECT().Push(gomock.Any()).
					Do(func(t *transactionState) {
						Expect(t.action).To(Equal(writeBufferFetch))
						Expect(t.fetchPID).To(Equal(vm.PID(1)))
						Expect(t.fetchAddress).To(Equal(uint64(0x100)))
					})
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(m.mshrState.Entries).To(HaveLen(1))
			})
		})

		Context("miss, mshr miss, need eviction", func() {
			BeforeEach(func() {
				// Fill all blocks in the target set with dirty data
				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				for i := 0; i < m.wayAssociativity; i++ {
					block := &m.directoryState.Sets[setID].Blocks[i]
					block.PID = 2
					block.Tag = uint64(0x200 + i*0x1000)
					block.CacheAddress = uint64(i * m.blockSize)
					block.IsValid = true
					block.IsDirty = true
				}
			})

			It("should stall if bank buffer is full", func() {
				bankBuf.EXPECT().CanPush().Return(false)

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})

			It("should do evict", func() {
				bankBuf.EXPECT().CanPush().Return(true)
				bankBuf.EXPECT().
					Push(gomock.Any()).
					Do(func(t *transactionState) {
						Expect(t.hasVictim).To(BeTrue())
						Expect(t.evictingPID).To(Equal(vm.PID(2)))
						Expect(t.fetchPID).To(Equal(vm.PID(1)))
						Expect(t.fetchAddress).To(Equal(uint64(0x100)))
					})
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankEvictAndFetch))
			})
		})
	})

	Context("write", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
		)

		BeforeEach(func() {
			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x100
			write.PID = 1
			write.TrafficBytes = 12
			write.TrafficClass = "mem.WriteReq"
			trans = &transactionState{
				write: write,
			}
			m.inFlightTransactions = []*transactionState{trans}

			pipeline.EXPECT().CanAccept().Return(false)
			buf.EXPECT().Peek().Return(dirPipelineItem{trans: trans})
			buf.EXPECT().Peek().Return(nil)
		})

		Context("mshr hit", func() {
			BeforeEach(func() {
				cache.MSHRAdd(&m.mshrState, m.numMSHREntry, vm.PID(1), 0x100)
			})

			It("should add to MSHR", func() {
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(m.mshrState.Entries[0].TransactionIndices).To(HaveLen(1))
			})
		})

		Context("hit", func() {
			BeforeEach(func() {
				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				block := &m.directoryState.Sets[setID].Blocks[0]
				block.Tag = 0x100
				block.PID = 1
				block.IsValid = true
			})

			It("should stall if bank is busy", func() {
				bankBuf.EXPECT().CanPush().Return(false)

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})

			It("should stall if block is locked", func() {
				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				m.directoryState.Sets[setID].Blocks[0].IsLocked = true

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})

			It("should send to bank", func() {
				bankBuf.EXPECT().CanPush().Return(true)
				bankBuf.EXPECT().Push(gomock.Any()).
					Do(func(t *transactionState) {
						Expect(t.hasBlock).To(BeTrue())
					})
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankWriteHit))
			})
		})

		Context("miss, write full line, no eviction", func() {
			BeforeEach(func() {
				write.Data = make([]byte, 64)
			})

			It("should stall if bank is busy", func() {
				bankBuf.EXPECT().CanPush().Return(false)

				ret := ds.Tick()

				Expect(ret).To(BeFalse())
			})

			It("should send to bank", func() {
				bankBuf.EXPECT().CanPush().Return(true)
				bankBuf.EXPECT().Push(gomock.Any()).
					Do(func(t *transactionState) {
						Expect(t.hasBlock).To(BeTrue())
					})
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankWriteHit))
			})
		})

		Context("miss, write full line, need eviction", func() {
			BeforeEach(func() {
				write.Data = make([]byte, 64)

				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				for i := 0; i < m.wayAssociativity; i++ {
					block := &m.directoryState.Sets[setID].Blocks[i]
					block.Tag = uint64(0x200 + i*0x1000)
					block.CacheAddress = uint64(i * m.blockSize)
					block.IsValid = true
					block.IsDirty = true
				}
			})

			It("should stall if bank buffer is full", func() {
				bankBuf.EXPECT().CanPush().Return(false)
				ret := ds.Tick()
				Expect(ret).To(BeFalse())
			})

			It("should send to evictor", func() {
				bankBuf.EXPECT().CanPush().Return(true)
				bankBuf.EXPECT().
					Push(gomock.Any()).
					Do(func(t *transactionState) {
						Expect(t.hasVictim).To(BeTrue())
					})
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankEvictAndWrite))
			})
		})

		Context("miss, write partial line, need eviction", func() {
			BeforeEach(func() {
				write.Data = make([]byte, 4)

				setID := int(0x100 / uint64(m.blockSize) % uint64(m.numSets))
				for i := 0; i < m.wayAssociativity; i++ {
					block := &m.directoryState.Sets[setID].Blocks[i]
					block.Tag = uint64(0x200 + i*0x1000)
					block.CacheAddress = uint64(i * m.blockSize)
					block.IsValid = true
					block.IsDirty = true
				}
			})

			It("should stall if mshr is full", func() {
				for i := 0; i < m.numMSHREntry; i++ {
					cache.MSHRAdd(&m.mshrState, m.numMSHREntry, vm.PID(100), uint64(i*0x1000))
				}
				ret := ds.Tick()
				Expect(ret).To(BeFalse())
			})

			It("should stall if bank buffer is full", func() {
				bankBuf.EXPECT().CanPush().Return(false)
				ret := ds.Tick()
				Expect(ret).To(BeFalse())
			})

			It("should evict and fetch", func() {
				bankBuf.EXPECT().CanPush().Return(true)
				bankBuf.EXPECT().
					Push(gomock.Any()).
					Do(func(t *transactionState) {
						Expect(t.hasVictim).To(BeTrue())
					})
				buf.EXPECT().Pop()

				ret := ds.Tick()

				Expect(ret).To(BeTrue())
				Expect(trans.action).To(Equal(bankEvictAndFetch))
			})
		})
	})
})
