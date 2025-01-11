package mesh_test

import (
	. "github.com/onsi/ginkgo/v2"
	"go.uber.org/mock/gomock"

	// . "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/noc/networking/mesh"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

var _ = Describe("Connector", func() {
	var (
		mockCtrl  *gomock.Controller
		engine    timing.Engine
		sim       *MockSimulation
		connector *mesh.Connector
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()

		sim = NewMockSimulation(mockCtrl)
		sim.EXPECT().GetEngine().Return(engine).AnyTimes()
		sim.EXPECT().RegisterStateHolder(gomock.Any()).AnyTimes()

		connector = mesh.NewConnector().WithSimulation(sim)
		connector.CreateNetwork("Network")
	})

	It("should be able to connect ports outside current capacity", func() {
		port := modeling.PortBuilder{}.
			WithSimulation(sim).
			WithIncomingBufCap(1).
			WithOutgoingBufCap(1).
			Build("Port")

		// 8,8,2 is the default capacity
		connector.AddTile([3]int{8, 8, 2}, []modeling.Port{port})

		connector.EstablishNetwork()
	})
})
