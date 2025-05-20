package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Flusher", func() {
	var (
		mockCtrl            *gomock.Controller
		controlPort         *MockPort
		topPort             *MockPort
		bottomPort          *MockPort
		directory           *MockDirectory
		dirBuf              *MockBuffer
		bankBuf             *MockBuffer
		mshrStageBuf        *MockBuffer
		writeBufferBuf      *MockBuffer
		mshr                *MockMSHR
		cacheModule         *Comp
		f                   *flusher
		addressToPortMapper *MockAddressToPortMapper
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("ControlPort")).
			AnyTimes()
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()

		directory = NewMockDirectory(mockCtrl)
		directory.EXPECT().WayAssociativity().Return(2).AnyTimes()
		dirBuf = NewMockBuffer(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		mshrStageBuf = NewMockBuffer(mockCtrl)
		writeBufferBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)

		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)

		builder := MakeBuilder().
			WithAddressToPortMapper(addressToPortMapper)
		cacheModule = builder.Build("Cache")
		cacheModule.topPort = topPort
		cacheModule.bottomPort = bottomPort
		cacheModule.controlPort = controlPort
		cacheModule.directory = directory
		cacheModule.mshr = mshr
		cacheModule.dirStageBuffer = dirBuf
		cacheModule.dirToBankBuffers = []sim.Buffer{bankBuf}
		cacheModule.mshrStageBuffer = mshrStageBuf
		cacheModule.writeBufferBuffer = writeBufferBuf
		cacheModule.dirStage = &directoryStage{
			cache:    cacheModule,
			pipeline: NewMockPipeline(mockCtrl),
			buf:      NewMockBuffer(mockCtrl),
		}
		cacheModule.mshrStage = &mshrStage{cache: cacheModule}

		f = &flusher{cache: cacheModule}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		controlPort.EXPECT().PeekIncoming().Return(nil)
		ret := f.Tick()
		Expect(ret).To(BeFalse())
	})

	Context("flush without reset", func() {
		It("should start flushing", func() {
			req := cache.FlushReqBuilder{}.Build()
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.processingFlush).To(BeIdenticalTo(req))
			Expect(cacheModule.state).To(Equal(cacheStatePreFlushing))
		})

		It("should do nothing if there is inflight transaction", func() {
			cacheModule.state = cacheStatePreFlushing
			cacheModule.inFlightTransactions = append(
				cacheModule.inFlightTransactions, &transaction{})
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should move to flush stage if no inflight transaction", func() {
			cacheModule.state = cacheStatePreFlushing
			cacheModule.inFlightTransactions = nil
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req

			sets := []cache.Set{
				{Blocks: []*cache.Block{
					{IsDirty: true, IsValid: true},
					{IsDirty: false, IsValid: true},
				}},
				{Blocks: []*cache.Block{
					{IsDirty: true, IsValid: false},
					{IsDirty: false, IsValid: false},
				}},
			}
			directory.EXPECT().GetSets().Return(sets)

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(cacheModule.state).To(Equal(cacheStateFlushing))
			Expect(f.blockToEvict).To(HaveLen(1))
		})

		It("should stall if bank buffer is full", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req

			blocks := []*cache.Block{{Tag: 0x0}, {Tag: 0x40}}
			f.blockToEvict = []*cache.Block{blocks[0], blocks[1]}

			bankBuf.EXPECT().CanPush().Return(false)

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should send read for eviction to bank", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req

			blocks := []*cache.Block{
				{
					Tag: 0x80,
					DirtyMask: []bool{
						true, true, false, false, true, true, false, false,
						true, true, false, false, true, true, false, false,
						true, true, false, false, true, true, false, false,
						true, true, false, false, true, true, false, false,
						true, true, false, false, true, true, false, false,
						true, true, false, false, true, true, false, false,
						true, true, false, false, true, true, false, false,
						true, true, false, false, true, true, false, false,
					},
				},
				{Tag: 0x40}}
			f.blockToEvict = []*cache.Block{blocks[0], blocks[1]}

			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).Do(func(trans *transaction) {
				Expect(trans.action).To(Equal(bankEvict))
				Expect(trans.evictingAddr).To(Equal(uint64(0x80)))
				Expect(trans.evictingDirtyMask).To(Equal(blocks[0].DirtyMask))
			})

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.blockToEvict).NotTo(ContainElement(blocks[0]))
			Expect(f.blockToEvict).To(ContainElement(blocks[1]))
		})

		It("should wait for bank buffer", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req
			f.blockToEvict = []*cache.Block{}

			bankBuf.EXPECT().Size().Return(1)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait for bank stage", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req
			f.blockToEvict = []*cache.Block{}

			bankBuf.EXPECT().Size().Return(0)
			cacheModule.bankStages[0].inflightTransCount = 1

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait for write buffer buffer", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req
			f.blockToEvict = []*cache.Block{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(1)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait for write buffer", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req
			f.blockToEvict = []*cache.Block{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(0)
			cacheModule.writeBuffer.inflightEviction = make([]*transaction, 1)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is controlPort sender is busy", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req
			f.blockToEvict = []*cache.Block{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(0)

			controlPort.EXPECT().CanSend().Return(false)

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should send response if all the blocks are evicted", func() {
			cacheModule.state = cacheStateFlushing
			req := cache.FlushReqBuilder{}.Build()
			f.processingFlush = req
			f.blockToEvict = []*cache.Block{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(0)
			mshr.EXPECT().Reset()
			directory.EXPECT().Reset()
			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any()).
				Do(func(rsp *cache.FlushRsp) {
					Expect(rsp.RspTo).To(Equal(req.ID))
				})

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.processingFlush).To(BeNil())
			Expect(cacheModule.state).To(Equal(cacheStateRunning))
		})
	})

	Context("flush with reset", func() {
		It("should remove inflight state", func() {
			req := cache.FlushReqBuilder{}.
				DiscardInflight().
				Build()
			sets := []cache.Set{
				{Blocks: []*cache.Block{
					{IsDirty: true, IsValid: true, IsLocked: true},
					{IsDirty: false, IsValid: true},
				}},
				{Blocks: []*cache.Block{
					{IsDirty: true, IsValid: false},
					{IsDirty: false, IsValid: false},
				}},
			}

			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			directory.EXPECT().GetSets().Return(sets)
			bankBuf.EXPECT().Clear()
			dirBuf.EXPECT().Clear()
			cacheModule.dirStage.pipeline.(*MockPipeline).EXPECT().Clear()
			cacheModule.dirStage.buf.(*MockBuffer).EXPECT().Clear()
			mshrStageBuf.EXPECT().Clear()
			writeBufferBuf.EXPECT().Clear()
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			bottomPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			// bottomPortSender.EXPECT().Clear()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.processingFlush).To(BeIdenticalTo(req))
			Expect(cacheModule.state).To(Equal(cacheStatePreFlushing))
			Expect(sets[0].Blocks[0].IsLocked).To(BeFalse())
		})
	})

	Context("restarting", func() {
		It("should stall if cannot send to control port", func() {
			req := cache.RestartReqBuilder{}.Build()
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().CanSend().Return(false)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should restart", func() {
			req := cache.RestartReqBuilder{}.Build()
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any())
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			bottomPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(cacheModule.state).To(Equal(cacheStateRunning))
		})
	})
})
