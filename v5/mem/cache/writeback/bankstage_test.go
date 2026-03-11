package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"go.uber.org/mock/gomock"

	"github.com/sarchlab/akita/v5/sim"
)

var _ = Describe("Bank Stage", func() {
	var (
		mockCtrl            *gomock.Controller
		m                   *middleware
		pipeline            *MockPipeline
		postPipelineBuf     queueing.Buffer
		dirInBuf            *MockBuffer
		writeBufferInBuf    *MockBuffer
		bs                  *bankStage
		storage             *mem.Storage
		writeBufferBuffer   *MockBuffer
		mshrStageBuffer     *MockBuffer
		addressToPortMapper *MockAddressToPortMapper
		topPort             *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		pipeline = NewMockPipeline(mockCtrl)
		postPipelineBuf = queueing.NewBuffer("Test.PostPipelineBuf", 2)
		dirInBuf = NewMockBuffer(mockCtrl)
		writeBufferInBuf = NewMockBuffer(mockCtrl)
		mshrStageBuffer = NewMockBuffer(mockCtrl)
		writeBufferBuffer = NewMockBuffer(mockCtrl)
		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)
		storage = mem.NewStorage(4 * mem.KB)

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		comp := MakeBuilder().
			WithEngine(sim.NewSerialEngine()).
			WithAddressToPortMapper(addressToPortMapper).
			WithTopPort(sim.NewPort(nil, 2, 2, "Cache.ToTop")).
			WithBottomPort(sim.NewPort(nil, 2, 2, "Cache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 2, 2, "Cache.ControlPort")).
			Build("Cache")
		m = comp.Middlewares()[0].(*middleware)

		m.dirToBankBuffers = []queueing.Buffer{dirInBuf}
		m.writeBufferToBankBuffers =
			[]queueing.Buffer{writeBufferInBuf}
		m.mshrStageBuffer = mshrStageBuffer
		m.writeBufferBuffer = writeBufferBuffer
		m.addressToPortMapper = addressToPortMapper
		m.storage = storage
		m.inFlightTransactions = nil
		m.topPort = topPort

		bs = &bankStage{
			cache:           m,
			bankID:          0,
			pipeline:        pipeline,
			pipelineWidth:   4,
			postPipelineBuf: postPipelineBuf,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("No transaction running", func() {
		It("should do nothing if pipeline is full", func() {
			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(false)

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should do nothing if there is no transaction", func() {
			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(true)
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			dirInBuf.EXPECT().Pop().Return(nil)

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should extract transactions from write buffer first", func() {
			trans := &transactionState{}

			pipeline.EXPECT().Tick()
			writeBufferInBuf.EXPECT().Pop().Return(trans)
			pipeline.EXPECT().CanAccept().Return(true)
			pipeline.EXPECT().Accept(gomock.Any())
			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(1))
		})

		It("should stall if write buffer buffer is full", func() {
			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(true)
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(false)

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should extract transactions from directory", func() {
			trans := &transactionState{}

			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(true)
			pipeline.EXPECT().Accept(gomock.Any())
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			dirInBuf.EXPECT().Pop().Return(trans)

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(1))
		})

		It("should directly forward fetch transaction to writebuffer", func() {
			trans := &transactionState{
				action: writeBufferFetch,
			}

			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(true)
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			writeBufferBuffer.EXPECT().Push(trans)
			dirInBuf.EXPECT().Pop().Return(trans)
			ret := bs.Tick()

			Expect(ret).To(BeTrue())
		})
	})

	Context("completing a read hit transaction", func() {
		var (
			read  *mem.ReadReq
			trans *transactionState
		)

		BeforeEach(func() {
			// Set up directory state with a block at set 0, way 0
			cache.DirectoryReset(&m.directoryState, 64, 4, 64)
			block := &m.directoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1

			storage.Write(0x40, []byte{1, 2, 3, 4, 5, 6, 7, 8})

			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
			read.Address = 0x104
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "mem.ReadReq"
			trans = &transactionState{
				read:       read,
				blockSetID: 0,
				blockWayID: 0,
				hasBlock:   true,
				action:     bankReadHit,
			}
			postPipelineBuf.Push(bankPipelineElem{trans: trans})
			m.inFlightTransactions = append(
				m.inFlightTransactions, trans)

			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if send buffer is full", func() {
			topPort.EXPECT().CanSend().Return(false)

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should read and send response", func() {
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					dr := msg.(*mem.DataReadyRsp)
					Expect(dr.Meta().RspTo).To(Equal(read.ID))
					Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				})

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			block := &m.directoryState.Sets[0].Blocks[0]
			Expect(block.ReadCount).To(Equal(0))
			Expect(m.inFlightTransactions).
				NotTo(ContainElement(trans))
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})

	Context("completing a write-hit transaction", func() {
		var (
			write *mem.WriteReq
			trans *transactionState
		)

		BeforeEach(func() {
			cache.DirectoryReset(&m.directoryState, 64, 4, 64)
			block := &m.directoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.ReadCount = 1
			block.IsLocked = true

			write = &mem.WriteReq{}
			write.ID = sim.GetIDGenerator().Generate()
			write.Address = 0x104
			write.Data = []byte{5, 6, 7, 8}
			write.TrafficBytes = len([]byte{5, 6, 7, 8}) + 12
			write.TrafficClass = "mem.WriteReq"
			trans = &transactionState{
				write:      write,
				blockSetID: 0,
				blockWayID: 0,
				hasBlock:   true,
				action:     bankWriteHit,
			}
			m.inFlightTransactions = append(
				m.inFlightTransactions, trans)
			postPipelineBuf.Push(bankPipelineElem{trans: trans})
			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if send buffer is full", func() {
			topPort.EXPECT().CanSend().Return(false)

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should write and send response", func() {
			topPort.EXPECT().CanSend().Return(true)
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(write.ID))
				})

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			data, _ := storage.Read(0x44, 4)
			Expect(data).To(Equal([]byte{5, 6, 7, 8}))
			block := &m.directoryState.Sets[0].Blocks[0]
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsDirty).To(BeTrue())
			Expect(block.DirtyMask).To(Equal([]bool{
				false, false, false, false, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}))
			Expect(m.inFlightTransactions).
				NotTo(ContainElement(trans))
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})

	Context("completing a write fetched transaction", func() {
		var (
			trans *transactionState
		)

		BeforeEach(func() {
			cache.DirectoryReset(&m.directoryState, 64, 4, 64)
			block := &m.directoryState.Sets[0].Blocks[0]
			block.CacheAddress = 0x40
			block.IsLocked = true

			m.mshrState.Entries = append(m.mshrState.Entries, cache.MSHREntryState{
				BlockSetID: 0,
				BlockWayID: 0,
				HasBlock:   true,
				Data: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
			})

			trans = &transactionState{
				mshrEntryIndex: 0,
				hasMSHREntry:   true,
				action:         bankWriteFetched,
			}
			postPipelineBuf.Push(bankPipelineElem{trans: trans})

			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if the mshr stage buffer is full", func() {
			mshrStageBuffer.EXPECT().CanPush().Return(false)

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should write to storage and send to mshr stage", func() {
			mshrStageBuffer.EXPECT().CanPush().Return(true)
			mshrStageBuffer.EXPECT().Push(0) // mshr entry index

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			writtenData, _ := storage.Read(0x40, 64)
			Expect(writtenData).To(Equal(m.mshrState.Entries[0].Data))
			block := &m.directoryState.Sets[0].Blocks[0]
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsValid).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})

	Context("finalizing a read for eviction action", func() {
		var (
			trans *transactionState
		)

		BeforeEach(func() {
			trans = &transactionState{
				hasVictim:          true,
				victimTag:          0x200,
				victimCacheAddress: 0x300,
				victimDirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
				action:       bankEvictAndFetch,
				evictingAddr: 0x200,
			}
			postPipelineBuf.Push(bankPipelineElem{trans: trans})
			pipeline.EXPECT().Tick()
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if the bottom sender is busy", func() {
			writeBufferBuffer.EXPECT().CanPush().Return(false)

			ret := bs.Tick()

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should send write to bottom", func() {
			data := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			storage.Write(0x300, data)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			writeBufferBuffer.EXPECT().Push(gomock.Any()).
				Do(func(eviction *transactionState) {
					Expect(eviction.action).To(Equal(writeBufferEvictAndFetch))
					Expect(eviction.evictingData).To(Equal(data))
				})

			ret := bs.Tick()

			Expect(ret).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})
})
