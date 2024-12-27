package cache

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/tagging"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

var _ = Describe("Read", func() {
	var (
		mockCtrl   *gomock.Controller
		sim        *MockSimulation
		cache      *Comp
		mshr       *MockMSHR
		topPort    *MockPort
		bottomPort *MockPort
		read       *defaultReadStrategy
		req        mem.ReadReq
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().GetEngine().Return(nil).AnyTimes()
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

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

		cache = MakeBuilder().
			WithSimulation(sim).
			Build("Cache")
		cache.MSHR = mshr
		cache.topPort = topPort
		cache.bottomPort = bottomPort

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
		Expect(cache.EvictQueue.Size()).To(Equal(0))
	})

	It("should handle read miss, w/o eviction", func() {
		expectTopPortPeekAndRetrieve(topPort, req)
		expectMSHRMissAndNotFull(mshr, req)
		expectMSHRAddEntryAndReq(mshr, req)
		expectBottomPortCanSendAndSend(bottomPort)

		read.Tick()

		Expect(read.state.Transactions).To(HaveLen(1))
		Expect(read.state.Transactions[0].req).To(Equal(req))
	})

	It("should handle read miss, w/ eviction", func() {
		fillSetWithDirtyBlocks(cache, 0)

		expectTopPortPeekAndRetrieve(topPort, req)
		expectMSHRMissAndNotFull(mshr, req)
		expectMSHRAddEntryAndReq(mshr, req)
		expectBottomPortCanSendAndSend(bottomPort)

		read.Tick()

		Expect(read.state.Transactions).To(HaveLen(1))
		Expect(read.state.Transactions[0].req).To(Equal(req))
		Expect(cache.EvictQueue.Size()).To(Equal(1))
	},
	)

	It("should handle read hit", func() {
		fillWithHitBlock(cache, req)
		expectTopPortPeekAndRetrieve(topPort, req)
		expectMSHRMissAndNotFull(mshr, req)

		read.Tick()

		Expect(read.state.Transactions).To(HaveLen(1))
		Expect(read.state.Transactions[0].req).To(Equal(req))
		Expect(cache.TopDownPreStorageBuffer.Size()).To(Equal(1))

		trans := cache.TopDownPreStorageBuffer.Peek().(*transaction)
		Expect(trans).To(Equal(&transaction{
			req:       req,
			transType: transactionTypeReadHit,
			setID:     0,
			wayID:     0,
		}))
	})
})

func fillSetWithDirtyBlocks(cache *Comp, setID int) {
	for wayID := 0; wayID < cache.Tags.NumWays; wayID++ {
		cache.Tags.Update(tagging.Block{
			SetID:    setID,
			WayID:    wayID,
			IsValid:  true,
			IsDirty:  true,
			IsLocked: false,
		})
	}
}

func fillWithHitBlock(cache *Comp, req mem.ReadReq) {
	cache.Tags.Update(tagging.Block{
		SetID:    0,
		WayID:    0,
		IsValid:  true,
		IsDirty:  false,
		IsLocked: false,
		Tag:      req.Address,
		PID:      req.PID,
	})
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
