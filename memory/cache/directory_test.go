package cache_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/syifan/yaotsu/memory/cache"
)

var _ = Describe("Directory", func() {
	d := cache.NewDirectory()
	d.BlockSize = 64 // 0x40
	d.NumSets = 1024
	d.NumWays = 4 // 0x04
	// 0x40 * 0x04 = 0x100 -- The size of a set

	It("should be able to get total size", func() {
		Expect(d.TotalSize()).To(Equal(uint64(262144)))
	})

	It("should find empty spot at beginning", func() {
		block := d.FindEmpty(0x40)
		Expect(block.SetID).To(Equal(uint(1)))
		Expect(block.WayID).To(Equal(uint(0)))
		Expect(block.CacheAddress).To(Equal(uint64(0x100)))
		block.Tag = 0x40
		block.IsValid = true
	})

	It("should be able to lookup occupied block", func() {
		block := d.Lookup(0x40)
		Expect(block).NotTo(BeNil())
		Expect(block.CacheAddress).To(Equal(uint64(0x100)))
	})

	It("should not be able to lookup unoccupied block", func() {
		block := d.Lookup(0x80)
		Expect(block).To(BeNil())

		block = d.Lookup(0x140)
		Expect(block).To(BeNil())
	})

	It("should be 4-way associative", func() {
		block := d.FindEmpty(0x10040)
		Expect(block.CacheAddress).To(Equal(uint64(0x140)))
		block.IsValid = true
		block.Tag = 0x10040

		block = d.FindEmpty(0x20040)
		Expect(block.CacheAddress).To(Equal(uint64(0x180)))
		block.IsValid = true
		block.Tag = 0x20040

		block = d.FindEmpty(0x40040)
		Expect(block.CacheAddress).To(Equal(uint64(0x1C0)))
		block.IsValid = true
		block.Tag = 0x40040

		block = d.FindEmpty(0x50040)
		Expect(block).To(BeNil())
	})

	It("should reuse evicted blocks", func() {
		block := d.Lookup(0x10040)
		block.IsValid = false

		block = d.FindEmpty(0x50040)
		Expect(block.CacheAddress).To(Equal(uint64(0x140)))
	})

})
