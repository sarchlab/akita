package dram_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/dram"
)

var _ = Describe("DRAM Statistics", func() {
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
})
