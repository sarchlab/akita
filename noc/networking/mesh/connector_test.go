package mesh_test

import (
	. "github.com/onsi/ginkgo/v2"
	// . "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/noc/networking/mesh"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

var _ = Describe("Connector", func() {
	var (
		engine    timing.Engine
		connector *mesh.Connector
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		connector = mesh.NewConnector().WithEngine(engine)
		connector.CreateNetwork("Network")
	})

	It("should be able to connect ports outside current capacity", func() {
		port := modeling.NewPort(nil, 1, 1, "Port")

		// 8,8,2 is the default capacity
		connector.AddTile([3]int{8, 8, 2}, []modeling.Port{port})

		connector.EstablishNetwork()
	})
})
