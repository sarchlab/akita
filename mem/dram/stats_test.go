package dram_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(name string, bufSize int) messaging.Port {
	p := messaging.NewPort(name, bufSize, bufSize)
	p.SetOwner(testDriver{})

	return p
}

var _ = Describe("DRAM Statistics", func() {
	// Unit tests for stat computation functions
	It("should compute row buffer hit rate", func() {
		c := &dram.Comp{}
		c.State.RowBufferHits = 3
		c.State.RowBufferMisses = 7
		Expect(dram.RowBufferHitRate(c)).To(
			BeNumerically("~", 0.3, 0.001))
	})

	It("should return 0 hit rate when no accesses", func() {
		c := &dram.Comp{}
		Expect(dram.RowBufferHitRate(c)).To(Equal(0.0))
	})

	It("should compute average read latency", func() {
		c := &dram.Comp{}
		c.State.CompletedReads = 10
		c.State.TotalReadLatencyCycles = 200
		Expect(dram.AverageReadLatency(c)).To(
			BeNumerically("~", 20.0, 0.001))
	})

	It("should return 0 average read latency when no reads completed", func() {
		c := &dram.Comp{}
		Expect(dram.AverageReadLatency(c)).To(Equal(0.0))
	})

	It("should compute average write latency", func() {
		c := &dram.Comp{}
		c.State.CompletedWrites = 5
		c.State.TotalWriteLatencyCycles = 100
		Expect(dram.AverageWriteLatency(c)).To(
			BeNumerically("~", 20.0, 0.001))
	})

	It("should return 0 average write latency when no writes completed", func() {
		c := &dram.Comp{}
		Expect(dram.AverageWriteLatency(c)).To(Equal(0.0))
	})

	It("should compute read bandwidth", func() {
		c := &dram.Comp{}
		c.State.BytesRead = 1024
		c.State.TotalCycles = 100
		Expect(dram.ReadBandwidth(c)).To(
			BeNumerically("~", 10.24, 0.001))
	})

	It("should compute write bandwidth", func() {
		c := &dram.Comp{}
		c.State.BytesWritten = 2048
		c.State.TotalCycles = 200
		Expect(dram.WriteBandwidth(c)).To(
			BeNumerically("~", 10.24, 0.001))
	})

	It("should return 0 bandwidth when no cycles", func() {
		c := &dram.Comp{}
		Expect(dram.ReadBandwidth(c)).To(Equal(0.0))
		Expect(dram.WriteBandwidth(c)).To(Equal(0.0))
	})

	// Integration test: verify stats accumulate during real simulation
	It("should accumulate statistics during simulation", func() {
		engine := timing.NewSerialEngine()
		sim := modeling.NewStandaloneSimulation(engine)

		conn := directconnection.MakeBuilder().
			WithSimulation(sim).
			Build("StatsConn")

		spec := dram.Definition.DefaultSpec
		spec.Freq = 1 * timing.GHz
		dramComp := dram.Definition.Builder().
			WithSimulation(sim).
			WithSpec(spec).
			WithResources(dram.Resources{Storage: mem.NewStorage(4 * mem.GB)}).
			WithPorts(dram.Ports{
				Top:     messaging.NewPort("StatsDRAM.Top", 1024, 1024),
				Control: messaging.NewPort("StatsDRAM.Control", 1024, 1024),
			}).
			Build("StatsDRAM")

		topPort := dramComp.Ports.Top
		srcPort := newDriverPort("Src.Top", 1024)
		conn.PlugIn(topPort)
		conn.PlugIn(srcPort)

		// Send a write request
		write := messaging.Msg{Payload: memprotocol.WriteReq{
			Address: 0x40,
			Data:    []byte{1, 2, 3, 4}},
			ID: sim.NewID(),

			Src: srcPort.AsRemote(),
			Dst: topPort.AsRemote()}

		write.TrafficBytes = len(write.Payload.(memprotocol.WriteReq).Data) + 12
		write.TrafficClass = "memprotocol.WriteReq"
		srcPort.Send(write)

		// Send a read request
		read := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        0x40,
			AccessByteSize: 4},
			ID: sim.NewID(),

			Src:          srcPort.AsRemote(),
			Dst:          topPort.AsRemote(),
			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		srcPort.Send(read)

		Expect(engine.Run()).To(Succeed())

		state := &dramComp.State
		Expect(state.CompletedReads).To(BeNumerically(">=", 1))
		Expect(state.CompletedWrites).To(BeNumerically(">=", 1))
		Expect(state.TotalReadLatencyCycles).To(BeNumerically(">", 0))
		Expect(state.TotalWriteLatencyCycles).To(BeNumerically(">", 0))
		Expect(state.BytesRead).To(BeNumerically(">", 0))
		Expect(state.BytesWritten).To(BeNumerically(">", 0))
		Expect(state.TotalCycles).To(BeNumerically(">", 0))
		Expect(state.RowBufferHits + state.RowBufferMisses).To(
			BeNumerically(">", 0))
		Expect(dram.AverageReadLatency(dramComp)).To(BeNumerically(">", 0))
		Expect(dram.AverageWriteLatency(dramComp)).To(BeNumerically(">", 0))
		Expect(dram.ReadBandwidth(dramComp)).To(BeNumerically(">", 0))
		Expect(dram.WriteBandwidth(dramComp)).To(BeNumerically(">", 0))
	})
})
