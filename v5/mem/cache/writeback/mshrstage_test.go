package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/stateutil"
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
			Return(sim.RemotePort("TopPort")).
			AnyTimes()

		initialState := State{
			DirStageBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirStageBuf", Cap: 4,
			},
			DirToBankBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.DirToBankBuf", Cap: 4,
			}},
			WriteBufferToBankBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.WBToBankBuf", Cap: 4,
			}},
			MSHRStageBuf: stateutil.Buffer[int]{
				BufferName: "Cache.MSHRStageBuf", Cap: 4,
			},
			WriteBufferBuf: stateutil.Buffer[int]{
				BufferName: "Cache.WriteBufferBuf", Cap: 4,
			},
			DirPipeline: stateutil.Pipeline[int]{Width: 4, NumStages: 0},
			DirPostPipelineBuf: stateutil.Buffer[int]{
				BufferName: "Cache.DirPostBuf", Cap: 4,
			},
			BankPipelines: []stateutil.Pipeline[int]{{Width: 4, NumStages: 10}},
			BankPostPipelineBufs: []stateutil.Buffer[int]{{
				BufferName: "Cache.BankPostBuf", Cap: 4,
			}},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		m = &pipelineMW{
			topPort:      topPort,
			evictingList: make(map[uint64]bool),
		}
		m.comp = modeling.NewBuilder[Spec, State]().
			WithEngine(nil).
			WithFreq(1 * sim.GHz).
			WithSpec(Spec{
				Log2BlockSize:  6,
				NumReqPerCycle: 4,
			}).
			Build("Cache")

		m.comp.SetState(initialState)
		m.inFlightTransactions = nil

		ms = &mshrStage{
			cache: m,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no entry in input buffer", func() {
		m.syncForTest()

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
		trans := &transactionState{
			HasRead:            true,
			ReadMeta:           read.MsgMeta,
			ReadAddress:        read.Address,
			ReadAccessByteSize: read.AccessByteSize,
			ReadPID:            read.PID,
		}

		mshrTrans := &transactionState{
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

		m.inFlightTransactions = []*transactionState{trans, mshrTrans}

		// Push mshrTrans to the MSHR stage buffer
		next := m.comp.GetNextState()
		next.MSHRStageBuf.Elements = []int{1}

		topPort.EXPECT().CanSend().Return(false)

		m.syncForTest()

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
		trans := &transactionState{
			HasRead:            true,
			ReadMeta:           read.MsgMeta,
			ReadAddress:        read.Address,
			ReadAccessByteSize: read.AccessByteSize,
			ReadPID:            read.PID,
		}

		mshrTrans := &transactionState{
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
		m.inFlightTransactions = []*transactionState{trans, mshrTrans}

		next := m.comp.GetNextState()
		next.MSHRStageBuf.Elements = []int{1}

		topPort.EXPECT().CanSend().Return(true)
		topPort.EXPECT().Send(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		m.syncForTest()

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingTrans).To(BeFalse())
		Expect(m.inFlightTransactions).NotTo(ContainElement(trans))
	})

	It("should discard the request if it is no longer inflight", func() {
		mshrTrans := &transactionState{
			MSHRTransactionIndices: []int{99}, // index that doesn't exist or is nil
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

		m.inFlightTransactions = []*transactionState{mshrTrans}

		next := m.comp.GetNextState()
		next.MSHRStageBuf.Elements = []int{0}

		topPort.EXPECT().CanSend().Return(true)

		m.syncForTest()

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		Expect(ms.hasProcessingTrans).To(BeFalse())
	})
})
