package writeback

import (
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

		mshrTrans := &transactionState{
			mshrTransactions: []*transactionState{trans},
			mshrData: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}

		inBuf.EXPECT().Pop().Return(mshrTrans)
		topPort.EXPECT().CanSend().Return(false)

		ret := ms.Tick()

		Expect(ret).To(BeFalse())
		Expect(ms.hasProcessingTrans).To(BeTrue())
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

		mshrTrans := &transactionState{
			mshrTransactions: []*transactionState{trans},
			mshrData: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}
		inBuf.EXPECT().Pop().Return(mshrTrans)
		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingTrans).To(BeFalse())
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

		ms.hasProcessingTrans = true
		ms.processingTrans = &transactionState{}
		ms.processingTransList = []*transactionState{trans}
		ms.processingData = []byte{
			1, 2, 3, 4, 9, 9, 9, 9,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).
			Do(func(msg sim.Msg) {
				Expect(msg.Meta().RspTo).To(Equal(write.ID))
			})

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingTrans).To(BeFalse())
		Expect(m.inFlightTransactions).NotTo(ContainElement(trans))
	})

	It("should discard the request if it is no longer inflight", func() {
		// Create a "stale" transaction pointer that's not in inFlightTransactions
		staleTrans := &transactionState{}
		m.inFlightTransactions = nil

		mshrTrans := &transactionState{
			mshrTransactions: []*transactionState{staleTrans},
			mshrData: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}

		inBuf.EXPECT().Pop().Return(mshrTrans)
		topPort.EXPECT().CanSend().Return(true)

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingTrans).To(BeFalse())
	})
})
