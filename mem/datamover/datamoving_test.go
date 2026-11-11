package datamover

import (
	"bytes"
	"path/filepath"
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

func TestDataMover(t *testing.T) {
	var (
		engine         timing.Engine
		sim            *testsim.Simulation
		dataMover      *Comp
		insideMem      *idealmemcontroller.Comp
		insideStorage  *mem.Storage
		outsideMem     *idealmemcontroller.Comp
		outsideStorage *mem.Storage
		conn           *direct.Connection
		srcPort        messaging.Port
	)

	setup := func(t *testing.T, bufferSize uint64, limit int) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		srcPort = newDriverPort(sim, "Src.Top", 4)

		memSpec := idealmemcontroller.DefaultSpec()
		memSpec.Latency = 100
		memSpec.NumReqPerCycle = 1

		storage := mem.MakeStorageBuilder().WithSimulation(sim).WithCapacity(mem.MB)
		insideStorage = storage.Build("InsideStorage")
		insideMem = buildIdealMem(sim, memSpec, insideStorage, "InsideMem")
		outsideStorage = storage.Build("OutsideStorage")
		outsideMem = buildIdealMem(sim, memSpec, outsideStorage, "OutsideMem")

		dmSpec := DefaultSpec()
		dmSpec.MaxTransactions = limit
		dmSpec.BufferSize = bufferSize
		dmSpec.InsideByteGranularity = 64
		dmSpec.OutsideByteGranularity = 256

		dmSpec.Inside.Destinations = []messaging.RemotePort{insideMem.Ports.Top.AsRemote()}

		dmSpec.Outside.Destinations = []messaging.RemotePort{outsideMem.Ports.Top.AsRemote()}
		dataMover = modeling.MakeBuilder[*Comp](dmSpec).
			WithSimulation(sim).
			Build("DataMover")
		dataMoverPorts := makePorts(sim, "DataMover", 16, 64, 64, 40960000)
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

	setup1 := func(t *testing.T) { t.Helper(); setup(t, 2048, 1) }
	runStream := func(t *testing.T, limit int, restore bool) timing.VTimeInPicoSec {
		t.Helper()
		setup(t, 65536, limit)
		var ids [4]uint64
		var expected [4][]byte
		for i := range 4 {
			addr := uint64(i * 8192)
			data := make([]byte, 4096)
			for j := range data {
				data[j] = byte(i + j)
			}
			outsideStorage.Write(addr, data)
			expected[i] = data
			id := sim.NewID()
			ids[i] = id
			dataMover.Ports.Top.Deliver(messaging.Msg{ID: id, Src: srcPort.AsRemote(),
				Dst: dataMover.Ports.Top.AsRemote(), Payload: datamoverprotocol.Req{
					SrcAddress: addr, DstAddress: addr, SrcSide: "outside", DstSide: "inside", ByteSize: 4096,
				}})
		}
		if restore {
			require.NoError(t, engine.(*timing.SerialEngine).RunUntil(130000))
			require.Len(t, dataMover.State.Transactions, 4)
			require.NotEmpty(t, dataMover.State.Transactions[0].PendingWrite)
			require.NotEmpty(t, dataMover.State.Transactions[2].PendingRead)
			path := filepath.Join(t.TempDir(), "in-flight.chk")
			require.NoError(t, sim.SaveCheckpoint(path))
			setup(t, 65536, limit)
			require.NoError(t, sim.LoadCheckpoint(path))
		}
		require.NoError(t, engine.Run())
		for i, id := range ids {
			addr := uint64(i * 8192)
			require.Equal(t, expected[i], insideStorage.Read(addr, 4096))
			rsp, ok := srcPort.RetrieveIncoming()
			require.True(t, ok)
			require.Equal(t, id, rsp.RspTo)
		}
		return engine.CurrentTime()
	}
	t.Run("independent page stream", func(t *testing.T) {
		serial := runStream(t, 1, false)
		overlapped := runStream(t, 4, false)
		require.Equal(t, timing.VTimeInPicoSec(1070000), serial) // measured on the single-transaction implementation
		require.Less(t, overlapped, serial)
		require.Equal(t, overlapped, runStream(t, 4, true))
		t.Logf("four independent 4 KiB copies: %d ps serial, %d ps overlapped", serial, overlapped)
	})

	t.Run("short final chunks in either direction", func(t *testing.T) {
		for _, side := range []datamoverprotocol.Side{"inside", "outside"} {
			setup(t, 2048, 2)
			src, dst := insideStorage, outsideStorage
			dstSide := datamoverprotocol.SideOutside
			if side == "outside" {
				src, dst = dst, src
				dstSide = datamoverprotocol.SideInside
			}
			for i := range 2 {
				addr := uint64(4096 * i)
				src.Write(addr, bytes.Repeat([]byte{0x11}, 512))
				dst.Write(addr, bytes.Repeat([]byte{0xff}, 512))
				dataMover.Ports.Top.Deliver(messaging.Msg{ID: sim.NewID(), Src: srcPort.AsRemote(),
					Dst: dataMover.Ports.Top.AsRemote(), Payload: datamoverprotocol.Req{
						SrcSide: side, DstSide: dstSide, SrcAddress: addr, DstAddress: addr, ByteSize: 257,
					}})
			}
			require.NoError(t, engine.Run())
			for i := range 2 {
				addr := uint64(4096 * i)
				require.Equal(t, bytes.Repeat([]byte{0x11}, 257), dst.Read(addr, 257))
				require.Equal(t, []byte{0xff}, dst.Read(addr+257, 1))
				_, ok := srcPort.RetrieveIncoming()
				require.True(t, ok)
			}
		}
	})

	t.Run("should move data outside to inside", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 0,
			DstSide:    "inside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),
		}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, insideStorage.Read(0, 4096))
		value0, present0 := srcPort.RetrieveIncoming()
		require.True(t, present0)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value0)
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

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 4096,
			SrcSide:    "outside",
			DstAddress: 8192,
			DstSide:    "inside",
			ByteSize:   2048},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),
		}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, insideStorage.Read(8192, 2048))
		value1, present1 := srcPort.RetrieveIncoming()
		require.True(t, present1)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value1)
	})

	t.Run("should move data inside to outside", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 0,
			DstSide:    "outside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),
		}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, insideStorage.Read(0, 4096))
		value2, present2 := srcPort.RetrieveIncoming()
		require.True(t, present2)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value2)
	})

	t.Run("should move on difference addresses", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 4096)
		for i := range 4096 {
			data[i] = byte(i)
		}
		insideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 4096,
			DstSide:    "outside",
			ByteSize:   4096},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),
		}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		require.Equal(t, data, outsideStorage.Read(4096, 4096))
		value3, present3 := srcPort.RetrieveIncoming()
		require.True(t, present3)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value3)
	})

	t.Run("should move partial data", func(t *testing.T) {
		setup1(t)

		data := make([]byte, 1024)
		for i := range 1024 {
			data[i] = byte(i)
		}
		outsideStorage.Write(0, data)

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "outside",
			DstAddress: 512,
			DstSide:    "inside",
			ByteSize:   512},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),
		}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		expected := data[:512]
		require.Equal(t, expected, insideStorage.Read(512, 512))
		value4, present4 := srcPort.RetrieveIncoming()
		require.True(t, present4)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value4)
	})

	t.Run("should handle zero-size transfers", func(t *testing.T) {
		setup1(t)

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 0,
			DstSide:    "outside",
			ByteSize:   0},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),
		}

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

		req := messaging.Msg{Payload: datamoverprotocol.Req{
			SrcAddress: 0,
			SrcSide:    "inside",
			DstAddress: 512,
			DstSide:    "inside",
			ByteSize:   512},
			ID:  sim.NewID(),
			Src: srcPort.AsRemote(),
			Dst: dataMover.Ports.Top.AsRemote(),
		}

		dataMover.Ports.Top.Deliver(req)

		require.NoError(t, engine.Run())

		expected := append(data[:512], data[:512]...)
		require.Equal(t, expected, insideStorage.Read(0, 1024))
		value5, present5 := srcPort.RetrieveIncoming()
		require.True(t, present5)
		require.IsType(t, messaging.Msg{Payload: datamoverprotocol.Rsp{}}, value5)
	})
}
