package cache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"go.uber.org/mock/gomock"
)

var _ = Describe("BottomInteraction", func() {
	var (
		mockCtrl     *gomock.Controller
		sim          *MockSimulation
		cache        *Comp
		victimFinder *MockVictimFinder
		tagArray     *MockTagArray
		mshr         *MockMSHR
		topPort      *MockPort
		bottomPort   *MockPort
		m            *bottomInteraction
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().GetEngine().Return(nil).AnyTimes()
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

		tagArray = NewMockTagArray(mockCtrl)
		victimFinder = NewMockVictimFinder(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("TopPort")).
			AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("BottomPort")).
			AnyTimes()

		addrToDstTable := mem.SinglePortMapper{Port: "Remote"}
		cache = MakeBuilder().
			WithSimulation(sim).
			WithAddressToDstTable(addrToDstTable).
			Build("Cache")
		cache.mshr = mshr
		cache.topPort = topPort
		cache.bottomPort = bottomPort
		cache.tags = tagArray
		cache.victimFinder = victimFinder

		m = &bottomInteraction{Comp: cache}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send read request down", func() {
		trans := &transaction{
			transType: transactionTypeReadMiss,
			reqToBottom: mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID: "123",
				},
			},
		}
		cache.bottomInteractionBuf.Push(trans)

		bottomPort.EXPECT().PeekIncoming().Return(nil)
		bottomPort.EXPECT().Send(trans.reqToBottom).Return(nil)

		madeProgress := m.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(cache.bottomInteractionBuf.Size()).To(Equal(0))
	})

	It("should pass DataReady to storage", func() {
		trans := &transaction{
			transType: transactionTypeReadMiss,
			reqToBottom: mem.ReadReq{
				MsgMeta: modeling.MsgMeta{
					ID: "123",
				},
			},
		}
		cache.Transactions = append(cache.Transactions, trans)

		dr := mem.DataReadyRsp{
			MsgMeta: modeling.MsgMeta{
				ID: "124",
			},
			Data:      []byte{1, 2, 3, 4},
			RespondTo: "123",
		}

		bottomPort.EXPECT().PeekIncoming().Return(dr)
		bottomPort.EXPECT().RetrieveIncoming()

		madeProgress := m.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(cache.storageBottomUpBuf.Size()).To(Equal(1))
		Expect(cache.storageBottomUpBuf.Peek()).To(BeIdenticalTo(trans))
		Expect(trans.rspFromBottom).To(Equal(dr))
	})
})
