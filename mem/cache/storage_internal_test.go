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

var _ = Describe("Storage", func() {
	var (
		mockCtrl   *gomock.Controller
		sim        *MockSimulation
		cache      *Comp
		mshr       *MockMSHR
		topPort    *MockPort
		bottomPort *MockPort
		storage    *storageMiddleware
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
		cache.mshr = mshr
		cache.topPort = topPort
		cache.bottomPort = bottomPort

		storage = &storageMiddleware{
			Comp: cache,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should handle read hit", func() {
		req := mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				ID: "123",
			},
			Address:        0x1000,
			AccessByteSize: 4,
		}

		transaction := &transaction{
			req:   req,
			setID: 0,
			wayID: 0,
		}

		block := tagging.Block{
			PID:          vm.PID(0),
			Tag:          0x1000,
			WayID:        0,
			SetID:        0,
			CacheAddress: 0x40,
			IsValid:      true,
			IsDirty:      false,
			ReadCount:    0,
			IsLocked:     true,
			DirtyMask:    []bool{},
		}
		cache.tags.Update(block)

		cache.storagePostPipelineBuf.Push(transaction)

		topPort.EXPECT().
			Send(gomock.Any()).
			Return(nil).
			AnyTimes()

		storage.processPostPipelineBuffer()

		Expect(storage.state.Transactions).To(HaveLen(1))
		Expect(storage.state.Transactions[0].req).To(Equal(req))

		finalBlock, ok := cache.tags.Lookup(vm.PID(0), 0x1000)
		Expect(ok).To(BeTrue())
		Expect(finalBlock.IsLocked).To(BeFalse())
	})
})
