package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("WriteBufferStage", func() {
	var (
		mockCtrl            *gomock.Controller
		m                   *middleware
		wb                  *writeBufferStage
		writeBufferBuf      *MockBuffer
		bottomPort          *MockPort
		bankBuf             *MockBuffer
		addressToPortMapper *MockAddressToPortMapper
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		writeBufferBuf = NewMockBuffer(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("BottomPort")).
			AnyTimes()
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

		m.writeBufferBuffer = writeBufferBuf
		m.bottomPort = bottomPort
		m.writeBufferToBankBuffers = []queueing.Buffer{bankBuf}
		m.addressToPortMapper = addressToPortMapper

		wb = &writeBufferStage{
			cache:               m,
			writeBufferCapacity: 16,
			maxInflightFetch:    4,
			maxInflightEviction: 4,
		}
		m.writeBuffer = wb
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transactions", func() {
		writeBufferBuf.EXPECT().Peek().Return(nil)
		bottomPort.EXPECT().PeekIncoming().Return(nil)

		ret := wb.Tick()

		Expect(ret).To(BeFalse())
	})

	Context("processing new writeBufferFetch transactions", func() {
		It("should fetch from bottom", func() {
			trans := &transactionState{
				action:       writeBufferFetch,
				fetchAddress: 0x100,
				fetchPID:     1,
				blockSetID:   0,
				blockWayID:   0,
				hasBlock:     true,
				read: &mem.ReadReq{},
			}
			trans.read.ID = sim.GetIDGenerator().Generate()
			trans.read.TrafficClass = "mem.ReadReq"

			writeBufferBuf.EXPECT().Peek().Return(trans)
			bottomPort.EXPECT().PeekIncoming().Return(nil)
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().Send(gomock.Any())
			writeBufferBuf.EXPECT().Pop()
			addressToPortMapper.EXPECT().
				Find(uint64(0x100)).
				Return(sim.RemotePort("DRAM"))

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.inflightFetch).To(HaveLen(1))
		})

		It("should stall fetch if too many inflight fetches", func() {
			wb.inflightFetch = make([]*transactionState, 4)

			trans := &transactionState{
				action:       writeBufferFetch,
				fetchAddress: 0x100,
				read: &mem.ReadReq{},
			}
			trans.read.ID = sim.GetIDGenerator().Generate()
			trans.read.TrafficClass = "mem.ReadReq"

			writeBufferBuf.EXPECT().Peek().Return(trans)
			bottomPort.EXPECT().PeekIncoming().Return(nil)

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
		})
	})

	Context("writing evictions", func() {
		It("should send eviction to bottom", func() {
			trans := &transactionState{
				evictingAddr: 0x200,
				evictingPID:  2,
				evictingData: make([]byte, 64),
				read: &mem.ReadReq{},
			}
			trans.read.ID = sim.GetIDGenerator().Generate()
			trans.read.TrafficClass = "mem.ReadReq"
			wb.pendingEvictions = []*transactionState{trans}

			writeBufferBuf.EXPECT().Peek().Return(nil)
			bottomPort.EXPECT().PeekIncoming().Return(nil)
			bottomPort.EXPECT().CanSend().Return(true)
			bottomPort.EXPECT().Send(gomock.Any())
			addressToPortMapper.EXPECT().
				Find(uint64(0x200)).
				Return(sim.RemotePort("DRAM"))

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.pendingEvictions).To(HaveLen(0))
			Expect(wb.inflightEviction).To(HaveLen(1))
		})

		It("should stall if too many inflight evictions", func() {
			wb.inflightEviction = make([]*transactionState, 4)
			wb.pendingEvictions = []*transactionState{{}}

			writeBufferBuf.EXPECT().Peek().Return(nil)
			bottomPort.EXPECT().PeekIncoming().Return(nil)

			ret := wb.Tick()

			Expect(ret).To(BeFalse())
		})
	})

	Context("processing responses", func() {
		It("should process write done response", func() {
			evictWrite := &mem.WriteReq{}
			evictWrite.ID = "write-1"
			evictWrite.TrafficClass = "mem.WriteReq"

			trans := &transactionState{
				evictionWriteReq: evictWrite,
				read: &mem.ReadReq{},
			}
			trans.read.ID = sim.GetIDGenerator().Generate()
			trans.read.TrafficClass = "mem.ReadReq"
			wb.inflightEviction = []*transactionState{trans}

			rsp := &mem.WriteDoneRsp{}
			rsp.RspTo = "write-1"
			rsp.TrafficClass = "mem.WriteDoneRsp"

			writeBufferBuf.EXPECT().Peek().Return(nil)
			bottomPort.EXPECT().PeekIncoming().Return(rsp)
			bottomPort.EXPECT().RetrieveIncoming().Return(rsp)

			ret := wb.Tick()

			Expect(ret).To(BeTrue())
			Expect(wb.inflightEviction).To(HaveLen(0))
		})
	})
})
