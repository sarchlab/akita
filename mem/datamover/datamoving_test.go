package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestDataMover(t *testing.T) {
	var (
		engine         timing.Engine
		sim            timing.Simulation
		dataMover      *Comp
		insideMem      *idealmemcontroller.Comp
		insideStorage  *mem.Storage
		outsideMem     *idealmemcontroller.Comp
		outsideStorage *mem.Storage
		conn           *direct.Connection
		srcPort        messaging.Port
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		srcPort = newDriverPort("Src.Top", 4)

		memSpec := idealmemcontroller.DefaultSpec()
		memSpec.Latency = 100
		memSpec.Width = 1
		memSpec.CacheLineSize = 64

		insideStorage = mem.NewStorage(1 * mem.MB)
		insideMem = buildIdealMem(sim, memSpec, insideStorage, "InsideMem")
		outsideStorage = mem.NewStorage(1 * mem.MB)
		outsideMem = buildIdealMem(sim, memSpec, outsideStorage, "OutsideMem")

		dmSpec := DefaultSpec()
		dmSpec.BufferSize = 2048
		dmSpec.InsideByteGranularity = 64
		dmSpec.OutsideByteGranularity = 256

		dmSpec.Inside.Destinations = []messaging.RemotePort{insideMem.Ports.Top.AsRemote()}

		dmSpec.Outside.Destinations = []messaging.RemotePort{outsideMem.Ports.Top.AsRemote()}
		dataMover = modeling.MakeBuilder[*Comp](dmSpec).
			WithSimulation(sim).
			Build("DataMover")
		dataMoverPorts := makePorts("DataMover", 16, 64, 64, 40960000)
		dataMover.BindPort("Top", dataMoverPorts.Top)
		dataMover.BindPort("Inside", dataMoverPorts.Inside)
		dataMover.BindPort("Outside", dataMoverPorts.Outside)
		dataMover.BindPort("Control", dataMoverPorts.Control)

		conn = direct.NewConnection("Conn", sim, timing.GHz)
		conn.BindPort(srcPort)
		conn.BindPort(dataMover.Ports.Top)
		conn.BindPort(dataMover.Ports.Inside)
		conn.BindPort(dataMover.Ports.Outside)
		conn.BindPort(insideMem.Ports.Top)
		conn.BindPort(outsideMem.Ports.Top)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	t.Run("should move data outside to inside", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, insideStorage.Read(0, 4096))
		value0, present0 := srcPort.RetrieveIncoming()
		require.True(t, present0)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.DataMoveRsp{}}, value0)
	})

	t.Run("should move data when SrcAddress is beyond BufferSize", func(t *testing.T) {
		setup1(t)

		// Regression: the read-window check must use the transaction-relative
		// address. With a source address at or beyond BufferSize (2048 here),
		// an absolute-address check would reject every read and the move would
		// hang. Source and destination addresses also differ.
		data := make([]byte, 2048)
		for i := range 2048 {
			data[i] = byte(i)
		}
		outsideStorage.Write(4096, data)

		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 4096,
			SrcSide:    "outside",
			DstAddress: 8192,
			DstSide:    "inside",
			ByteSize:   2048},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, insideStorage.Read(8192, 2048))
		value1, present1 := srcPort.RetrieveIncoming()
		require.True(t, present1)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.DataMoveRsp{}}, value1)
	})

	t.Run("should move data inside to outside", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 0,
			DstSide:    "outside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, insideStorage.Read(0, 4096))
		value2, present2 := srcPort.RetrieveIncoming()
		require.True(t, present2)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.DataMoveRsp{}}, value2)
	})

	t.Run("should move on difference addresses", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 4096,
			DstSide:    "outside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, outsideStorage.Read(4096, 4096))
		value3, present3 := srcPort.RetrieveIncoming()
		require.True(t, present3)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.DataMoveRsp{}}, value3)
	})

	t.Run("should move partial data", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 512,
			DstSide:    "inside",
			ByteSize:   512},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		expected := data[:512]
		require.Equal(t, expected, insideStorage.Read(512, 512))
		value4, present4 := srcPort.RetrieveIncoming()
		require.True(t, present4)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.DataMoveRsp{}}, value4)
	})

	t.Run("should handle zero-size transfers", func(t *testing.T) {
		setup1(t)

		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 0,
			DstSide:    "outside",
			ByteSize:   0},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		require.NotPanics(t, func() {
			dataMover.Ports.Top.Deliver(req)
		})
	})

	t.Run("should handle overlapping ranges", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 512,
			DstSide:    "inside",
			ByteSize:   512},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),

			TrafficClass: "datamoverprotocol.DataMoveReq"}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		expected := append(data[:512], data[:512]...)
		require.Equal(t, expected, insideStorage.Read(0, 1024))
		value5, present5 := srcPort.RetrieveIncoming()
		require.True(t, present5)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.DataMoveRsp{}}, value5)
	})
}
