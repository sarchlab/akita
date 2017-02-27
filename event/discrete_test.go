package event_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/core/event"
)

var _ = Describe("Freq", func() {
	It("should get period", func() {
		var f event.Freq = 1 * event.GHz
		fmt.Println(event.GHz)
		Expect(f.Period()).To(BeNumerically("==", 1e-9))
	})

	It("should get the next tick", func() {
		var f event.Freq = 1 * event.GHz
		Expect(f.NextTick(102.000000001)).To(BeNumerically("~", 102.000000002, 1e-12))
	})

	It("should get the next tick, if currTime is not on a tick", func() {
		var f event.Freq = 1 * event.GHz
		Expect(f.NextTick(102.0000000011)).To(BeNumerically("~", 102.000000002, 1e-12))
	})

	It("should get the n cycles later", func() {
		var f event.Freq = 1 * event.GHz
		Expect(f.NCyclesLater(12, 102.000000001)).To(BeNumerically("~", 102.000000014, 1e-12))
	})

	It("should get the n cycles later, if current time is not on a tick", func() {
		var f event.Freq = 1 * event.GHz
		Expect(f.NCyclesLater(12, 102.0000000011)).To(BeNumerically("~", 102.000000014, 1e-12))
	})

})
