package simulation

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
)

var _ = Describe("Simulation", func() {
	var (
		mockCtrl   *gomock.Controller
		simulation *Simulation
		comp       *MockComponent
		port       *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		simulation = NewSimulation()

		comp = NewMockComponent(mockCtrl)
		comp.EXPECT().Name().Return("comp").AnyTimes()

		port = NewMockPort(mockCtrl)
		port.EXPECT().Name().Return("port").AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should register a component", func() {
		comp.EXPECT().Ports().Return([]sim.Port{port})

		simulation.RegisterComponent(comp)

		Expect(simulation.GetComponentByName("comp")).To(Equal(comp))
		Expect(simulation.GetPortByName("port")).To(Equal(port))
	})
})
