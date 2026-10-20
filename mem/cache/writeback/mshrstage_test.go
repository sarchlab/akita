package writeback

import (
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/stretchr/testify/require"
)

func TestMSHRStage(t *testing.T) {
	var (
		m       *pipelineMW
		ms      *mshrStage
		topPort messaging.Port
	)

	// fillTop pre-fills topPort's single outgoing slot so the next CanSend
	// returns false, simulating a busy port.
	fillTop := func(t *testing.T) {
		t.Helper()

		dummy := messaging.Msg{ID: 1, Payload: memprotocol.DataReadyRsp{},
			Src: topPort.AsRemote(),
			Dst: messaging.RemotePort("SomeSrc"),
		}

		require.True(t, topPort.CanSend())
		topPort.Send(dummy)
	}

	setup1 := func(t *testing.T) {
		t.Helper()

		initialState := state{
			CacheState:   int(cacheStateRunning),
			EvictingList: make(map[uint64]bool),
			DirStageBuf:  queueing.MakeBuffer[int](4),
			DirToBankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			WriteBufferToBankBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			MSHRStageBuf:       queueing.MakeBuffer[int](4),
			WriteBufferBuf:     queueing.MakeBuffer[int](4),
			DirPipeline:        queueing.MakePipeline[int](4, 0),
			DirPostPipelineBuf: queueing.MakeBuffer[int](4),
			BankPipelines: []queueing.Pipeline[int]{
				queueing.MakePipeline[int](4, 10),
			},
			BankPostPipelineBufs: []queueing.Buffer[int]{
				queueing.MakeBuffer[int](4),
			},
			BankInflightTransCounts:         []int{0},
			BankDownwardInflightTransCounts: []int{0},
		}

		// The stage sends on the Top port, so the test gives it a single-slot
		// Top port to simulate a busy port.
		spec := stageTestSpec()
		sim := akitasim.MakeBuilder().WithTaskTracking().Build()
		ports := makePorts(sim, "Cache", 4)
		ports.Top = twowaybuffered.MakeBuilder().WithSimulation(sim).Build()
		comp := buildStageTestComp(sim, spec,
			Resources{Storage: mem.NewStorage(spec.ByteSize())}, ports)
		topPort = comp.Ports.Top

		m = comp.Middlewares.Pipeline
		m.comp.State = initialState

		ms = m.mshrStage
	}

	t.Run("should do nothing if there is no entry in input buffer", func(t *testing.T) {
		setup1(t)

		ret := ms.Tick()
		require.False(t, ret)
	})

	t.Run("should stall if topSender is busy", func(t *testing.T) {
		setup1(t)

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x104,
			AccessByteSize: 4},
			ID: m.comp.NewID(),

			TrafficBytes: 12,
		}

		trans := transactionState{
			HasRead:            true,
			ReadMeta:           read,
			ReadAddress:        read.Payload.(memprotocol.ReadReq).Address,
			ReadAccessByteSize: read.Payload.(memprotocol.ReadReq).AccessByteSize,
			ReadPID:            read.Payload.(memprotocol.ReadReq).PID,
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

		fillTop(t)

		ret := ms.Tick()

		require.False(t, ret)
		next = &m.comp.State
		require.True(t, next.HasProcessingMSHREntry)
	})

	t.Run("should send data ready to top", func(t *testing.T) {
		setup1(t)

		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x104,
			AccessByteSize: 4},
			ID:  m.comp.NewID(),
			Src: messaging.RemotePort("Agent"),

			TrafficBytes: 12,
		}

		trans := transactionState{
			HasRead:            true,
			ReadMeta:           read,
			ReadAddress:        read.Payload.(memprotocol.ReadReq).Address,
			ReadAccessByteSize: read.Payload.(memprotocol.ReadReq).AccessByteSize,
			ReadPID:            read.Payload.(memprotocol.ReadReq).PID,
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

		require.True(t, ret)
		next = &m.comp.State
		require.False(t, next.HasProcessingMSHREntry)
		require.True(t, next.Transactions[0].Removed)

		out, _ := topPort.RetrieveOutgoing()
		dr := out
		require.Equal(t, []byte{5, 6, 7, 8}, dr.Payload.(memprotocol.DataReadyRsp).Data)
	})

	t.Run("should discard the request if it is no longer inflight", func(t *testing.T) {
		setup1(t)

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

		require.True(t, ret)
		next = &m.comp.State
		require.False(t, next.HasProcessingMSHREntry)
	})
}
