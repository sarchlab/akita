package simulation

// import (
// 	"github.com/golang/mock/gomock"
// 	. "github.com/onsi/ginkgo/v2"
// 	// . "github.com/onsi/gomega"
// )

// var _ = Describe("Simulation", func() {
// 	var (
// 		mockCtrl *gomock.Controller
// 		sim      *Simulation
// 		comp     *MockComponent
// 		port     *MockPort
// 	)

// 	BeforeEach(func() {
// 		mockCtrl = gomock.NewController(GinkgoT())
// 		sim = NewSimulation()

// 		comp = NewMockComponent(mockCtrl)
// 		comp.EXPECT().Name().Return("comp").AnyTimes()

// 		port = NewMockPort(mockCtrl)
// 		port.EXPECT().Name().Return("port").AnyTimes()
// 	})

// 	AfterEach(func() {
// 		mockCtrl.Finish()
// 	})

// 	// It("should register a component", func() {
// 	// 	comp.EXPECT().Ports().Return([]modeling.Port{port})

// 	// 	sim.RegisterComponent(comp)

// 	// 	Expect(sim.GetComponentByName("comp")).To(Equal(comp))
// 	// 	Expect(sim.GetPortByName("port")).To(Equal(port))
// 	// })
// })
