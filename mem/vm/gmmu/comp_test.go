package gmmu_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v4/mem/vm/gmmu"
	"github.com/sarchlab/akita/v4/sim"
)

var _ = Describe("Builder", func() {
	It("creates ports with defaults", func() {
		engine := sim.NewSerialEngine()

		unit := gmmu.MakeBuilder().
			WithEngine(engine).
			Build("GMMU")

		Expect(unit.GetPortByName("Top")).NotTo(BeNil())
		Expect(unit.GetPortByName("Bottom")).NotTo(BeNil())
	})
})
