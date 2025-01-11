package cache

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/tagging"
	vm "github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

var _ = Describe("Read", func() {
	var (
		mockCtrl     *gomock.Controller
		sim          *MockSimulation
		cache        *Comp
		victimFinder *MockVictimFinder
		tagArray     *MockTagArray
		mshr         *MockMSHR
		topPort      *MockPort
		bottomPort   *MockPort
		read         *defaultReadStrategy
		req          mem.ReadReq
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
		read = &defaultReadStrategy{
			Comp: cache,
		}

		req = mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				ID: "123",
			},
			Address:        0x1000,
			AccessByteSize: 4,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should handle read MSHR hit", func() {
		expectTopPortPeekAndRetrieve(topPort, req)
		expectMSHRHitAndAddReq(mshr, req)

		read.Tick()

		Expect(read.state.Transactions).To(HaveLen(1))
		Expect(read.state.Transactions[0].req).To(Equal(req))
		Expect(cache.storageTopDownBuf.Size()).To(Equal(0))
	})

	It("should handle read miss, w/o eviction", func() {
		expectTopPortPeekAndRetrieve(topPort, req)
		expectVictimFinderFindCleanVictim(victimFinder)
		expectTagArrayMiss(tagArray)
		expectMSHRMissAndNotFull(mshr, req)
		expectMSHRAddEntryAndReq(mshr, req)

		read.Tick()

		Expect(read.state.Transactions).To(HaveLen(1))
		Expect(read.state.Transactions[0].req).To(Equal(req))
		Expect(cache.bottomInteractionBuf.Size()).To(Equal(1))
	})

	It("should handle read miss, w/ eviction", func() {
		expectTagArrayMiss(tagArray)
		expectVictimFinderFindDirtyVictim(victimFinder)
		expectTopPortPeekAndRetrieve(topPort, req)
		expectMSHRMissAndNotFull(mshr, req)
		expectMSHRAddEntryAndReq(mshr, req)
		expectBottomPortCanSendAndSend(bottomPort)

		read.Tick()

		Expect(read.state.Transactions).To(HaveLen(1))
		Expect(read.state.Transactions[0].req).To(Equal(req))
		Expect(cache.storageTopDownBuf.Size()).To(Equal(1))
		Expect(cache.bottomInteractionBuf.Size()).To(Equal(1))
	})

	It("should handle read hit", func() {
		expectTagArrayHit(tagArray, 0, 0)
		expectTopPortPeekAndRetrieve(topPort, req)
		expectMSHRMissAndNotFull(mshr, req)

		read.Tick()

		Expect(read.state.Transactions).To(HaveLen(1))
		Expect(read.state.Transactions[0].req).To(Equal(req))
		Expect(cache.storageBottomUpBuf.Size()).To(Equal(1))

		trans := cache.storageBottomUpBuf.Peek().(*transaction)
		Expect(trans).To(Equal(&transaction{
			req:       req,
			transType: transactionTypeReadHit,
		}))
	})
})

func expectTagArrayMiss(tagArray *MockTagArray) {
	tagArray.EXPECT().
		Lookup(vm.PID(0), uint64(0x1000)).
		Return(tagging.Block{}, false)
	tagArray.EXPECT().
		Visit(gomock.Any()).
		Return()
}

func expectTagArrayHit(tagArray *MockTagArray, setID int, wayID int) {
	block := tagging.Block{
		SetID: setID,
		WayID: wayID,
	}

	tagArray.EXPECT().Lookup(vm.PID(0), uint64(0x1000)).Return(block, true)
	tagArray.EXPECT().Visit(gomock.Any())
}

func expectVictimFinderFindCleanVictim(victimFinder *MockVictimFinder) {
	block := tagging.Block{
		IsValid:  true,
		IsDirty:  false,
		IsLocked: false,
	}

	victimFinder.EXPECT().
		FindVictim(gomock.Any(), gomock.Any()).
		Return(block, true).
		AnyTimes()
}

func expectVictimFinderFindDirtyVictim(victimFinder *MockVictimFinder) {
	block := tagging.Block{
		IsValid:  true,
		IsDirty:  true,
		IsLocked: false,
	}

	victimFinder.EXPECT().
		FindVictim(gomock.Any(), gomock.Any()).
		Return(block, true).
		AnyTimes()
}

func expectMSHRMissAndNotFull(mshr *MockMSHR, req mem.ReadReq) {
	mshr.EXPECT().
		Lookup(req.PID, req.Address).
		Return(false).
		AnyTimes()
	mshr.EXPECT().
		IsFull().
		Return(false).
		AnyTimes()
}

func expectMSHRHitAndAddReq(mshr *MockMSHR, req mem.ReadReq) {
	mshr.EXPECT().
		Lookup(req.PID, req.Address).
		Return(true).
		AnyTimes()
	mshr.EXPECT().
		AddReqToEntry(req).
		Return(nil).
		AnyTimes()
}

func expectMSHRAddEntryAndReq(mshr *MockMSHR, req mem.ReadReq) {
	mshr.EXPECT().
		AddEntry(gomock.Any()).
		Return(nil).
		AnyTimes()
	mshr.EXPECT().
		AddReqToEntry(req).
		Return(nil).
		AnyTimes()
}

func expectBottomPortCanSendAndSend(bottomPort *MockPort) {
	bottomPort.EXPECT().
		CanSend().
		Return(true).
		AnyTimes()
	bottomPort.EXPECT().
		Send(gomock.Any()).
		Return(nil).
		AnyTimes()
}

func expectTopPortPeekAndRetrieve(topPort *MockPort, req mem.ReadReq) {
	topPort.EXPECT().
		PeekIncoming().
		Return(req).
		AnyTimes()
	topPort.EXPECT().
		RetrieveIncoming().
		Return(req).
		AnyTimes()
}
