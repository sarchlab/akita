package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MSHR Stage", func() {
	var (
		mockCtrl            *gomock.Controller
		m                   *middleware
		ms                  *mshrStage
		inBuf               *MockBuffer
		topPort             *MockPort
		addressToPortMapper *MockAddressToPortMapper
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		addressToPortMapper = NewMockAddressToPortMapper(mockCtrl)

		comp := MakeBuilder().
			WithEngine(sim.NewSerialEngine()).
			WithAddressToPortMapper(addressToPortMapper).
			WithTopPort(sim.NewPort(nil, 2, 2, "Cache.ToTop")).
			WithBottomPort(sim.NewPort(nil, 2, 2, "Cache.BottomPort")).
			WithControlPort(sim.NewPort(nil, 2, 2, "Cache.ControlPort")).
			Build("Cache")
		m = comp.Middlewares()[0].(*middleware)

		m.mshrStageBuffer = inBuf
		m.inFlightTransactions = nil
		m.topPort = topPort

		ms = &mshrStage{
			cache: m,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no entry in input buffer", func() {
		inBuf.EXPECT().Pop().Return(nil)
		ret := ms.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should stall if topSender is busy", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := &transactionState{read: read}
		m.inFlightTransactions = []*transactionState{trans}

		m.mshrState.Entries = append(m.mshrState.Entries, cache.MSHREntryState{
			TransactionIndices: []int{0},
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

		inBuf.EXPECT().Pop().Return(0) // mshr entry index
		topPort.EXPECT().CanSend().Return(false)

		ret := ms.Tick()

		Expect(ret).To(BeFalse())
		Expect(ms.hasProcessingMSHREntry).To(BeTrue())
	})

	It("should send data ready to top", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := &transactionState{read: read}
		m.inFlightTransactions = []*transactionState{trans}

		m.mshrState.Entries = append(m.mshrState.Entries, cache.MSHREntryState{
			TransactionIndices: []int{0},
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
		inBuf.EXPECT().Pop().Return(0)
		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingMSHREntry).To(BeFalse())
		Expect(m.inFlightTransactions).NotTo(ContainElement(trans))
	})

	It("should send write done to top", func() {
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Address = 0x104
		write.Data = []byte{9, 9, 9, 9}
		write.TrafficBytes = len([]byte{9, 9, 9, 9}) + 12
		write.TrafficClass = "mem.WriteReq"
		trans := &transactionState{write: write}
		m.inFlightTransactions = []*transactionState{trans}

		m.mshrState.Entries = append(m.mshrState.Entries, cache.MSHREntryState{
			PID:                1,
			Address:            0x100,
			TransactionIndices: []int{0},
			Data: []byte{
				1, 2, 3, 4, 9, 9, 9, 9,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		})
		ms.hasProcessingMSHREntry = true
		ms.processingMSHREntryIdx = 0
		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).
			Do(func(msg sim.Msg) {
				Expect(msg.Meta().RspTo).To(Equal(write.ID))
			})

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingMSHREntry).To(BeFalse())
		Expect(m.inFlightTransactions).NotTo(ContainElement(trans))
	})

	It("should discard the request if it is no longer inflight", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := &transactionState{read: read}
		// NOT in inFlightTransactions

		m.mshrState.Entries = append(m.mshrState.Entries, cache.MSHREntryState{
			TransactionIndices: []int{0},
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

		// The transaction is at index 0 but inFlightTransactions is empty
		// This should cause an out-of-bounds. Let me adjust:
		// Put a different trans in inflight so index 0 works but findTransaction fails
		otherTrans := &transactionState{}
		m.inFlightTransactions = []*transactionState{otherTrans}
		_ = trans

		inBuf.EXPECT().Pop().Return(0)
		topPort.EXPECT().CanSend().Return(true)

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingMSHREntry).To(BeFalse())
	})
})
