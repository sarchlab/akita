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
		tags       *MockTagArray
		topPort    *MockPort
		bottomPort *MockPort
		storage    *storageMiddleware

		readReq mem.ReadReq
		trans   *transaction
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().GetEngine().Return(nil).AnyTimes()
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

		mshr = NewMockMSHR(mockCtrl)
		tags = NewMockTagArray(mockCtrl)

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
		cache.tags = tags
		cache.topPort = topPort
		cache.bottomPort = bottomPort

		storage = &storageMiddleware{
			Comp: cache,
		}

		readReq = mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				ID: "123",
			},
			Address:        0x1000,
			AccessByteSize: 4,
		}

		trans = &transaction{
			block: tagging.Block{
				PID:          vm.PID(0),
				Tag:          0x1000,
				WayID:        0,
				SetID:        0,
				CacheAddress: 0x40,
				IsValid:      true,
				IsDirty:      false,
			},
		}

		cache.Transactions = append(cache.Transactions, trans)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should handle read hit", func() {
		trans.req = readReq
		trans.transType = transactionTypeReadHit

		prefillStorage(cache.storage)
		cache.storagePostPipelineBuf.Push(trans)

		expectDataReady(topPort, []byte{0, 1, 2, 3})
		tags.EXPECT().Unlock(0, 0)

		storage.processPostPipelineBuffer()

		Expect(storage.state.Transactions).To(HaveLen(0))
		Expect(cache.storagePostPipelineBuf.Size()).To(Equal(0))
	})

	It("should handle read hit, with offset", func() {
		readReq.Address = 0x1004
		trans.req = readReq
		trans.transType = transactionTypeReadHit

		prefillStorage(cache.storage)
		cache.storagePostPipelineBuf.Push(trans)

		expectDataReady(topPort, []byte{4, 5, 6, 7})
		tags.EXPECT().Unlock(0, 0)

		storage.processPostPipelineBuffer()

		Expect(storage.state.Transactions).To(HaveLen(0))
		Expect(cache.storagePostPipelineBuf.Size()).To(Equal(0))
	})
})

func prefillStorage(storage *mem.Storage) {
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	storage.Write(0x40, data)
}

func expectDataReady(port *MockPort, data []byte) {
	port.EXPECT().
		Send(gomock.Any()).
		DoAndReturn(func(msg modeling.Msg) error {
			Expect(msg).To(BeAssignableToTypeOf(mem.DataReadyRsp{}))
			Expect(msg.(mem.DataReadyRsp).Data).To(Equal(data))
			return nil
		}).
		AnyTimes()
}
