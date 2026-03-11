package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Flusher", func() {
	var (
		mockCtrl            *gomock.Controller
		controlPort         *MockPort
		topPort             *MockPort
		bottomPort          *MockPort
		dirBuf              *MockBuffer
		bankBuf             *MockBuffer
		mshrStageBuf        *MockBuffer
		writeBufferBuf      *MockBuffer
		m                   *middleware
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

		dirBuf = NewMockBuffer(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		mshrStageBuf = NewMockBuffer(mockCtrl)
		writeBufferBuf = NewMockBuffer(mockCtrl)

		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)

		comp := MakeBuilder().
			WithEngine(sim.NewSerialEngine()).
			WithAddressToPortMapper(addressToPortMapper).
			WithTopPort(sim.NewPort(nil, 2, 2, "Cache.ToTop")).
			WithBottomPort(sim.NewPort(nil, 2, 2, "Cache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 2, 2, "Cache.ControlPort")).
			Build("Cache")
		m = comp.Middlewares()[0].(*middleware)

		m.topPort = topPort
		m.bottomPort = bottomPort
		m.controlPort = controlPort
		m.dirStageBuffer = dirBuf
		m.dirToBankBuffers = []queueing.Buffer{bankBuf}
		m.mshrStageBuffer = mshrStageBuf
		m.writeBufferBuffer = writeBufferBuf
		m.dirStage = &directoryStage{
			cache:    m,
			pipeline: NewMockPipeline(mockCtrl),
			buf:      NewMockBuffer(mockCtrl),
		}
		m.mshrStage = &mshrStage{cache: m}

		f = &flusher{cache: m}
		m.flusher = f
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
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.processingFlush).To(BeIdenticalTo(req))
			Expect(m.state).To(Equal(cacheStatePreFlushing))
		})

		It("should do nothing if there is inflight transaction", func() {
			m.state = cacheStatePreFlushing
			m.inFlightTransactions = append(
				m.inFlightTransactions, &transactionState{})
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should move to flush stage if no inflight transaction", func() {
			m.state = cacheStatePreFlushing
			m.inFlightTransactions = nil
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req

			// Set up directory with one dirty valid block
			cache.DirectoryReset(&m.directoryState, 2, 2, 64)
			m.directoryState.Sets[0].Blocks[0].IsDirty = true
			m.directoryState.Sets[0].Blocks[0].IsValid = true

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(m.state).To(Equal(cacheStateFlushing))
			Expect(f.blockToEvict).To(HaveLen(1))
		})

		It("should stall if bank buffer is full", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req

			cache.DirectoryReset(&m.directoryState, 2, 2, 64)
			f.blockToEvict = []blockRef{{SetID: 0, WayID: 0}, {SetID: 1, WayID: 0}}

			bankBuf.EXPECT().CanPush().Return(false)

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should send read for eviction to bank", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req

			cache.DirectoryReset(&m.directoryState, 2, 2, 64)
			m.directoryState.Sets[0].Blocks[0].Tag = 0x80
			m.directoryState.Sets[0].Blocks[0].CacheAddress = 0
			m.directoryState.Sets[0].Blocks[0].DirtyMask = []bool{
				true, true, false, false, true, true, false, false,
				true, true, false, false, true, true, false, false,
				true, true, false, false, true, true, false, false,
				true, true, false, false, true, true, false, false,
				true, true, false, false, true, true, false, false,
				true, true, false, false, true, true, false, false,
				true, true, false, false, true, true, false, false,
				true, true, false, false, true, true, false, false,
			}
			m.directoryState.Sets[1].Blocks[0].Tag = 0x40
			f.blockToEvict = []blockRef{{SetID: 0, WayID: 0}, {SetID: 1, WayID: 0}}

			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).Do(func(trans *transactionState) {
				Expect(trans.action).To(Equal(bankEvict))
				Expect(trans.evictingAddr).To(Equal(uint64(0x80)))
			})

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.blockToEvict).To(HaveLen(1))
		})

		It("should wait for bank buffer", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req
			f.blockToEvict = []blockRef{}

			bankBuf.EXPECT().Size().Return(1)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait for bank stage", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req
			f.blockToEvict = []blockRef{}

			bankBuf.EXPECT().Size().Return(0)
			m.bankStages[0].inflightTransCount = 1

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait for write buffer buffer", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req
			f.blockToEvict = []blockRef{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(1)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait for write buffer", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req
			f.blockToEvict = []blockRef{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(0)
			m.writeBuffer.inflightEviction = make([]*transactionState, 1)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if controlPort sender is busy", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req
			f.blockToEvict = []blockRef{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(0)

			controlPort.EXPECT().CanSend().Return(false)

			ret := f.Tick()

			Expect(ret).To(BeFalse())
		})

		It("should send response if all the blocks are evicted", func() {
			m.state = cacheStateFlushing
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.FlushReq"
			f.processingFlush = req
			f.blockToEvict = []blockRef{}

			bankBuf.EXPECT().Size().Return(0)
			writeBufferBuf.EXPECT().Size().Return(0)
			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any()).
				Do(func(msg sim.Msg) {
					Expect(msg.Meta().RspTo).To(Equal(req.ID))
				})

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.processingFlush).To(BeNil())
			Expect(m.state).To(Equal(cacheStateRunning))
		})
	})

	Context("flush with reset", func() {
		It("should remove inflight state", func() {
			req := &cache.FlushReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.DiscardInflight = true
			req.TrafficClass = "cache.FlushReq"

			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			bankBuf.EXPECT().Clear()
			dirBuf.EXPECT().Clear()
			m.dirStage.pipeline.(*MockPipeline).EXPECT().Clear()
			m.dirStage.buf.(*MockBuffer).EXPECT().Clear()
			mshrStageBuf.EXPECT().Clear()
			writeBufferBuf.EXPECT().Clear()
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			ret := f.Tick()

			Expect(ret).To(BeTrue())
			Expect(f.processingFlush).To(BeIdenticalTo(req))
			Expect(m.state).To(Equal(cacheStatePreFlushing))
		})
	})

	Context("restarting", func() {
		It("should stall if cannot send to control port", func() {
			req := &cache.RestartReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.RestartReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().CanSend().Return(false)

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should restart", func() {
			req := &cache.RestartReq{}
			req.ID = sim.GetIDGenerator().Generate()
			req.TrafficClass = "cache.RestartReq"
			controlPort.EXPECT().PeekIncoming().Return(req)
			controlPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			controlPort.EXPECT().CanSend().Return(true)
			controlPort.EXPECT().Send(gomock.Any())
			topPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()
			bottomPort.EXPECT().RetrieveIncoming().Return(nil).AnyTimes()

			madeProgress := f.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(m.state).To(Equal(cacheStateRunning))
		})
	})
})
