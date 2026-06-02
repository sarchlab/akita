package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MSHR Stage", func() {
	var (
		mockCtrl *gomock.Controller
		m        *pipelineMW
		ms       *mshrStage
		topPort  *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().
			AsRemote().
			Return(messaging.RemotePort("TopPort")).
			AnyTimes()

		initialState := State{
			CacheState:   int(cacheStateRunning),
			EvictingList: make(map[uint64]bool),
			DirStageBuf:  queueing.NewBuffer[int]("Cache.DirStageBuf", 4),
			DirToBankBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.DirToBankBuf", 4),
			},
			WriteBufferToBankBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("Cache.WBToBankBuf", 4),
			},
			MSHRStageBuf:       queueing.NewBuffer[int]("Cache.MSHRStageBuf", 4),
			WriteBufferBuf:     queueing.NewBuffer[int]("Cache.WriteBufferBuf", 4),
			DirPipeline:        queueing.NewPipeline[int](4, 0),
			DirPostPipelineBuf: queueing.NewBuffer[int]("Cache.DirPostBuf", 4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.NewPipeline[int](4, 10),
			},
			BankPostPipelineBufs: []postPipelineBuf{
				newPostPipelineBuf(4),
			},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &pipelineMW{
			topPort: topPort,
		}
		m.comp = modeling.NewBuilder[Spec, State, Resources]().
			WithEngine(nil).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{
				Log2BlockSize:  6,
				NumReqPerCycle: 4,
			}).
			Build("Cache")

		m.comp.State = initialState

		ms = &mshrStage{
			cache: m,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no entry in input buffer", func() {

		ret := ms.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should stall if topSender is busy", func() {
		read := &mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := transactionState{
			HasRead:            true,
			ReadMeta:           read.MsgMeta,
			ReadAddress:        read.Address,
			ReadAccessByteSize: read.AccessByteSize,
			ReadPID:            read.PID,
		}

		mshrTrans := transactionState{
			MSHRTransactionIndices: []int{0},
			MSHRData: []byte{
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

		next := &m.comp.State
		next.Transactions = []transactionState{trans, mshrTrans}

		// Push mshrTrans to the MSHR stage buffer
		next.MSHRStageBuf.Clear()
		next.MSHRStageBuf.PushTyped(1)

		topPort.EXPECT().CanSend().Return(false)

		ret := ms.Tick()

		Expect(ret).To(BeFalse())
		next = &m.comp.State
		Expect(next.HasProcessingMSHREntry).To(BeTrue())
	})

	It("should send data ready to top", func() {
		read := &mem.ReadReq{}
		read.ID = timing.GetIDGenerator().Generate()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := transactionState{
			HasRead:            true,
			ReadMeta:           read.MsgMeta,
			ReadAddress:        read.Address,
			ReadAccessByteSize: read.AccessByteSize,
			ReadPID:            read.PID,
		}

		mshrTrans := transactionState{
			MSHRTransactionIndices: []int{0},
			MSHRData: []byte{
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

		next := &m.comp.State
		next.Transactions = []transactionState{trans, mshrTrans}
		next.MSHRStageBuf.Clear()
		next.MSHRStageBuf.PushTyped(1)

		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).
			Do(func(msg messaging.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		next = &m.comp.State
		Expect(next.HasProcessingMSHREntry).To(BeFalse())
		Expect(next.Transactions[0].Removed).To(BeTrue())
	})

	It("should discard the request if it is no longer inflight", func() {
		mshrTrans := transactionState{
			MSHRTransactionIndices: []int{99}, // index that doesn't exist
			MSHRData: []byte{
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

		next := &m.comp.State
		next.Transactions = []transactionState{mshrTrans}
		next.MSHRStageBuf.Clear()
		next.MSHRStageBuf.PushTyped(0)

		topPort.EXPECT().CanSend().Return(true)

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		next = &m.comp.State
		Expect(next.HasProcessingMSHREntry).To(BeFalse())
	})
})
