package simulation

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
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
		simulation = MakeBuilder().Build()

		comp = NewMockComponent(mockCtrl)
		comp.EXPECT().Name().Return("comp").AnyTimes()

		port = NewMockPort(mockCtrl)
		port.EXPECT().Name().Return("port").AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()

		simulation.Terminate()

		os.Remove("akita_sim_" + simulation.ID() + ".sqlite3")
	})

	It("should register a component", func() {
		comp.EXPECT().Ports().Return([]sim.Port{port}).AnyTimes()

		simulation.RegisterComponent(comp)

		Expect(simulation.GetComponentByName("comp")).To(Equal(comp))
		Expect(simulation.GetPortByName("port")).To(Equal(port))
	})
})
