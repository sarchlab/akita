package mesh_test

import (
	. "github.com/onsi/ginkgo/v2"
	// . "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/noc/networking/mesh"
	"github.com/sarchlab/akita/v4/sim"
)

var _ = Describe("Connector", func() {
	var (
		engine    sim.Engine
		connector *mesh.Connector
	)

	BeforeEach(func() {
		engine = sim.NewSerialEngine()
		connector = mesh.NewConnector().WithEngine(engine)
		connector.CreateNetwork("Network")
	})

	It("should be able to connect ports outside current capacity", func() {
		port := sim.NewPort(nil, 1, 1, "Port")

		// 8,8,2 is the default capacity
		connector.AddTile([3]int{8, 8, 2}, []sim.Port{port})

		connector.EstablishNetwork()
	})
})
