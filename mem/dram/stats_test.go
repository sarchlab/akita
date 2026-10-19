package dram_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(name string, bufSize int) messaging.Port {
	p := twowaybuffered.NewPort(bufSize, bufSize)
	p.BindOwner(testDriver{}, name)

	return p
}

func TestDRAMStatistics(t *testing.T) {
	// Unit tests for stat computation functions
	t.Run("should compute row buffer hit rate", func(t *testing.T) {
		c := &dram.Comp{}
		c.State.RowBufferHits = 3
		c.State.RowBufferMisses = 7
		require.InDelta(t, 0.3, dram.RowBufferHitRate(c), 0.001)
	})

	t.Run("should return 0 hit rate when no accesses", func(t *testing.T) {
		c := &dram.Comp{}
		require.Equal(t, 0.0, dram.RowBufferHitRate(c))
	})

	t.Run("should compute average read latency", func(t *testing.T) {
		c := &dram.Comp{}
		c.State.CompletedReads = 10
		c.State.TotalReadLatencyCycles = 200
		require.InDelta(t, 20.0, dram.AverageReadLatency(c), 0.001)
	})

	t.Run("should return 0 average read latency when no reads completed", func(t *testing.T) {
		c := &dram.Comp{}
		require.Equal(t, 0.0, dram.AverageReadLatency(c))
	})

	t.Run("should compute average write latency", func(t *testing.T) {
		c := &dram.Comp{}
		c.State.CompletedWrites = 5
		c.State.TotalWriteLatencyCycles = 100
		require.InDelta(t, 20.0, dram.AverageWriteLatency(c), 0.001)
	})

	t.Run("should return 0 average write latency when no writes completed", func(t *testing.T) {
		c := &dram.Comp{}
		require.Equal(t, 0.0, dram.AverageWriteLatency(c))
	})

	t.Run("should compute read bandwidth", func(t *testing.T) {
		c := &dram.Comp{}
		c.State.BytesRead = 1024
		c.State.TotalCycles = 100
		require.InDelta(t, 10.24, dram.ReadBandwidth(c), 0.001)
	})

	t.Run("should compute write bandwidth", func(t *testing.T) {
		c := &dram.Comp{}
		c.State.BytesWritten = 2048
		c.State.TotalCycles = 200
		require.InDelta(t, 10.24, dram.WriteBandwidth(c), 0.001)
	})

	t.Run("should return 0 bandwidth when no cycles", func(t *testing.T) {
		c := &dram.Comp{}
		require.Equal(t, 0.0, dram.ReadBandwidth(c))
		require.Equal(t, 0.0, dram.WriteBandwidth(c))
	})

	// Integration test: verify stats accumulate during real simulation
	t.Run("should accumulate statistics during simulation", func(t *testing.T) {
		sim := akitasim.MakeBuilder().Build()
		engine := sim.Engine().(*timing.SerialEngine)

		conn := direct.NewConnection("StatsConn", sim, timing.GHz)

		spec := dram.DefaultSpec()
		spec.Freq = 1 * timing.GHz
		dramComp := modeling.MakeBuilder[*dram.Comp](spec).
			WithSimulation(sim).
			WithResources(dram.Resources{Storage: mem.NewStorage(4 * mem.GB)}).
			Build("StatsDRAM")

		dramComp.BindPort("Top", twowaybuffered.NewPort(1024, 1024))
		dramComp.BindPort("Control", twowaybuffered.NewPort(1024, 1024))

		topPort := dramComp.Ports.Top
		srcPort := newDriverPort("Src.Top", 1024)
		conn.BindPort(topPort)
		conn.BindPort(srcPort)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}

		// Send a write request
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x40,
			Data:    []byte{1, 2, 3, 4}},
			ID: sim.NewID(),

			Src: srcPort.AsRemote(),
			Dst: topPort.AsRemote()}

		write.TrafficBytes = len(write.Payload.(memprotocol.WriteReq).Data) + 12
		srcPort.Send(write)

		// Send a read request
		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x40,
			AccessByteSize: 4},
			ID: sim.NewID(),

			Src:          srcPort.AsRemote(),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
		}

		srcPort.Send(read)

		require.NoError(t, engine.Run())

		state := &dramComp.State
		require.GreaterOrEqual(t, state.CompletedReads, uint64(1))
		require.GreaterOrEqual(t, state.CompletedWrites, uint64(1))
		require.Greater(t, state.TotalReadLatencyCycles, uint64(0))
		require.Greater(t, state.TotalWriteLatencyCycles, uint64(0))
		require.Greater(t, state.BytesRead, uint64(0))
		require.Greater(t, state.BytesWritten, uint64(0))
		require.Greater(t, state.TotalCycles, uint64(0))
		require.Greater(t, state.RowBufferHits+state.RowBufferMisses, uint64(0))
		require.Greater(t, dram.AverageReadLatency(dramComp), 0.0)
		require.Greater(t, dram.AverageWriteLatency(dramComp), 0.0)
		require.Greater(t, dram.ReadBandwidth(dramComp), 0.0)
		require.Greater(t, dram.WriteBandwidth(dramComp), 0.0)
	})
}

func (o testDriver) Name() string                           { return "TestDriver" }
func (o testDriver) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
