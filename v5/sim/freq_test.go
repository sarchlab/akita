package sim

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Freq", func() {
	It("should get period", func() {
		var f = 1 * GHz
		// 1 GHz → period = 1e12 / 1e9 = 1000 ps
		Expect(f.Period()).To(Equal(VTimeInSec(1000)))
	})

	It("should get this tick", func() {
		var f = 1 * Hz
		// 1 Hz → period = 1e12 ps
		// ThisTick(1) = ceil(1/1e12)*1e12 = 1e12
		Expect(f.ThisTick(1)).To(Equal(VTimeInSec(1000000000000)))
	})

	It("should get the next tick", func() {
		var f = 1 * GHz
		// period = 1000 ps
		// NextTick(102000) = (102000/1000 + 1)*1000 = 103000
		Expect(f.NextTick(102000)).To(Equal(VTimeInSec(103000)))
	})

	It("should get the next tick 2", func() {
		var f = 1 * GHz
		// NextTick(31000) = (31000/1000 + 1)*1000 = 32000
		Expect(f.NextTick(31000)).To(Equal(VTimeInSec(32000)))
	})

	It("should get the next tick 3", func() {
		var f = 1 * GHz
		// NextTick(17000) = (17000/1000 + 1)*1000 = 18000
		Expect(f.NextTick(17000)).To(Equal(VTimeInSec(18000)))
	})

	It("should get the next tick from on-boundary time", func() {
		var f = 1 * GHz
		// NextTick(16000) = (16000/1000 + 1)*1000 = 17000
		Expect(f.NextTick(16000)).To(Equal(VTimeInSec(17000)))
	})

	It("should get the next tick, if currTime is not on a tick", func() {
		var f = 1 * GHz
		// NextTick(102011) = (102011/1000 + 1)*1000 = 103000
		Expect(f.NextTick(102011)).To(Equal(VTimeInSec(103000)))
	})

	It("should get the n cycles later", func() {
		var f = 1 * GHz
		// NCyclesLater(12, 102000): ThisTick(102000)=102000, + 12*1000 = 114000
		Expect(f.NCyclesLater(12, 102000)).To(Equal(VTimeInSec(114000)))
	})

	It("should get the n cycles later, "+
		"if current time is not on a tick", func() {
		var f = 1 * GHz
		// NCyclesLater(12, 102011): ThisTick(102011)=103000, + 12*1000 = 115000
		Expect(f.NCyclesLater(12, 102011)).To(Equal(VTimeInSec(115000)))
	})

	It("should get the no-earlier-than time, on tick", func() {
		var f = 1 * GHz
		// NoEarlierThan(102000) = ThisTick(102000) = 102000
		Expect(f.NoEarlierThan(102000)).To(Equal(VTimeInSec(102000)))
	})

	It("should get the no-earlier-than time, off tick", func() {
		var f = 1 * GHz
		// NoEarlierThan(102011) = ThisTick(102011) = 103000
		Expect(f.NoEarlierThan(102011)).To(Equal(VTimeInSec(103000)))
	})

})
