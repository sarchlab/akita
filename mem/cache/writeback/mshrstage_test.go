package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
)

var _ = Describe("MSHR Stage", func() {
	var (
		m       *pipelineMW
		ms      *mshrStage
		topPort messaging.Port
	)

	// fillTop pre-fills topPort's single outgoing slot so the next CanSend
	// returns false, simulating a busy port.
	fillTop := func() {
		dummy := memprotocol.DataReadyRsp{}
		dummy.Src = topPort.AsRemote()
		dummy.Dst = messaging.RemotePort("SomeSrc")
		dummy.TrafficClass = "rsp"
		Expect(topPort.CanSend()).To(BeTrue())
		topPort.Send(dummy)
	}

	BeforeEach(func() {
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
			BankPostPipelineBufs: []queueing.Buffer[int]{
				queueing.NewBuffer[int]("BankPostPipelineBuf", 4),
			},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		// The stage sends on the Top port, so the test gives it a single-slot
		// Top port to simulate a busy port.
		spec := stageTestSpec()
		ports := makePorts("Cache", 4)
		ports.Top = messaging.NewPort(nil, 1, 1, "Cache.Top")
		comp := buildStageTestComp(spec,
			Resources{Storage: mem.NewStorage(spec.TotalByteSize)}, ports)
		topPort = comp.Ports.Top

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState

		ms = m.mshrStage
	})

	It("should do nothing if there is no entry in input buffer", func() {

		ret := ms.Tick()
		Expect(ret).To(BeFalse())
	})

	It("should stall if topSender is busy", func() {
		read := memprotocol.ReadReq{}
		read.ID = m.comp.NewID()
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"
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
		next.MSHRStageBuf.Push(1)

		fillTop()

		ret := ms.Tick()

		Expect(ret).To(BeFalse())
		next = &m.comp.State
		Expect(next.HasProcessingMSHREntry).To(BeTrue())
	})

	It("should send data ready to top", func() {
		read := memprotocol.ReadReq{}
		read.ID = m.comp.NewID()
		read.Src = messaging.RemotePort("Agent")
		read.Address = 0x104
		read.AccessByteSize = 4
		read.TrafficBytes = 12
		read.TrafficClass = "memprotocol.ReadReq"
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
		next.MSHRStageBuf.Push(1)

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		next = &m.comp.State
		Expect(next.HasProcessingMSHREntry).To(BeFalse())
		Expect(next.Transactions[0].Removed).To(BeTrue())

		out, _ := topPort.RetrieveOutgoing()
		dr := out.(memprotocol.DataReadyRsp)
		Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
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
		next.MSHRStageBuf.Push(0)

		ret := ms.Tick()

		Expect(ret).To(BeTrue())
		next = &m.comp.State
		Expect(next.HasProcessingMSHREntry).To(BeFalse())
	})
})
