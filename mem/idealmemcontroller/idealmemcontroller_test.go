package idealmemcontroller

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"

	. "github.com/onsi/gomega"
)

var _ = Describe("Ideal Memory Controller", func() {

	var (
		engine        sim.Engine
		memController *Comp
	)

	BeforeEach(func() {
		engine = sim.NewSerialEngine()
		
		memController = MakeBuilder().
			WithEngine(engine).
			WithNewStorage(1 * mem.MB).
			Build("MemCtrl")
		memController.Freq = 1000 * sim.MHz
		memController.Latency = 10
	})

	It("should have correct configuration", func() {
		Expect(memController.Name()).To(Equal("MemCtrl"))
		Expect(memController.Freq).To(Equal(1000 * sim.MHz))
		Expect(memController.Latency).To(Equal(10))
		Expect(memController.Storage).ToNot(BeNil())
	})

	It("should have top port", func() {
		topPort := memController.GetPortByName("Top")
		Expect(topPort).ToNot(BeNil())
		// Just check that the port name contains the expected parts
		Expect(topPort.Name()).To(ContainSubstring("MemCtrl"))
		Expect(topPort.Name()).To(ContainSubstring("Top"))
	})

	It("should support storage operations", func() {
		// Test storage write and read
		testData := []byte{1, 2, 3, 4}
		err := memController.Storage.Write(0x100, testData)
		Expect(err).To(BeNil())
		
		readData, err := memController.Storage.Read(0x100, 4)
		Expect(err).To(BeNil())
		Expect(readData).To(Equal(testData))
	})
})
