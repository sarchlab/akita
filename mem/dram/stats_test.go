package dram_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
)

var _ = Describe("DRAM Statistics", func() {
	// Unit tests for stat computation functions
	It("should compute row buffer hit rate", func() {
		state := &dram.State{}
		state.RowBufferHits = 3
		state.RowBufferMisses = 7
		Expect(dram.RowBufferHitRate(state)).To(
			BeNumerically("~", 0.3, 0.001))
	})

	It("should return 0 hit rate when no accesses", func() {
		state := &dram.State{}
		Expect(dram.RowBufferHitRate(state)).To(Equal(0.0))
	})

	It("should compute average read latency", func() {
		state := &dram.State{}
		state.CompletedReads = 10
		state.TotalReadLatencyCycles = 200
		Expect(dram.AverageReadLatency(state)).To(
			BeNumerically("~", 20.0, 0.001))
	})

	It("should return 0 average read latency when no reads completed", func() {
		state := &dram.State{}
		Expect(dram.AverageReadLatency(state)).To(Equal(0.0))
	})

	It("should compute average write latency", func() {
		state := &dram.State{}
		state.CompletedWrites = 5
		state.TotalWriteLatencyCycles = 100
		Expect(dram.AverageWriteLatency(state)).To(
			BeNumerically("~", 20.0, 0.001))
	})

	It("should return 0 average write latency when no writes completed", func() {
		state := &dram.State{}
		Expect(dram.AverageWriteLatency(state)).To(Equal(0.0))
	})

	It("should compute read bandwidth", func() {
		state := &dram.State{}
		state.BytesRead = 1024
		state.TotalCycles = 100
		Expect(dram.ReadBandwidth(state)).To(
			BeNumerically("~", 10.24, 0.001))
	})

	It("should compute write bandwidth", func() {
		state := &dram.State{}
		state.BytesWritten = 2048
		state.TotalCycles = 200
		Expect(dram.WriteBandwidth(state)).To(
			BeNumerically("~", 10.24, 0.001))
	})

	It("should return 0 bandwidth when no cycles", func() {
		state := &dram.State{}
		Expect(dram.ReadBandwidth(state)).To(Equal(0.0))
		Expect(dram.WriteBandwidth(state)).To(Equal(0.0))
	})

	// Integration test: verify stats accumulate during real simulation
	It("should accumulate statistics during simulation", func() {
		engine := sim.NewSerialEngine()
		conn := directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("StatsConn")

		topPort := sim.NewPort(nil, 1024, 1024, "StatsDRAM.Top")
		dramComp := dram.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			WithTopPort(topPort).
			Build("StatsDRAM")
		_ = dramComp

		srcPort := sim.NewPort(nil, 1024, 1024, "SrcPort")
		conn.PlugIn(topPort)
		conn.PlugIn(srcPort)

		// Send a write request
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Address = 0x40
		write.Data = []byte{1, 2, 3, 4}
		write.Src = srcPort.AsRemote()
		write.Dst = topPort.AsRemote()
		write.TrafficBytes = len(write.Data) + 12
		write.TrafficClass = "mem.WriteReq"
		srcPort.Send(write)

		// Send a read request
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x40
		read.AccessByteSize = 4
		read.Src = srcPort.AsRemote()
		read.Dst = topPort.AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		srcPort.Send(read)

		engine.Run()

		state := dramComp.GetNextState()
		Expect(state.CompletedReads).To(BeNumerically(">=", 1))
		Expect(state.CompletedWrites).To(BeNumerically(">=", 1))
		Expect(state.TotalReadLatencyCycles).To(BeNumerically(">", 0))
		Expect(state.TotalWriteLatencyCycles).To(BeNumerically(">", 0))
		Expect(state.BytesRead).To(BeNumerically(">", 0))
		Expect(state.BytesWritten).To(BeNumerically(">", 0))
		Expect(state.TotalCycles).To(BeNumerically(">", 0))
		Expect(state.RowBufferHits + state.RowBufferMisses).To(
			BeNumerically(">", 0))
		Expect(dram.AverageReadLatency(state)).To(BeNumerically(">", 0))
		Expect(dram.AverageWriteLatency(state)).To(BeNumerically(">", 0))
		Expect(dram.ReadBandwidth(state)).To(BeNumerically(">", 0))
		Expect(dram.WriteBandwidth(state)).To(BeNumerically(">", 0))
	})
})
