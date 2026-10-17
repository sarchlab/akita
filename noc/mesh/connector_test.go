package mesh_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	// . "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/noc/mesh"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

var _ = Describe("Connector", func() {
	var (
		engine    timing.Engine
		sim       timing.Simulation
		connector *mesh.Connector
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		connector = mesh.MakeBuilder().WithSimulation(sim).Build("Network")
	})

	It("should be able to connect ports outside current capacity", func() {
		port := twowaybuffered.NewPort(1, 1)
		port.BindOwner(testOwner{}, "Device.Port")

		// 8,8,2 is the default capacity
		connector.AddTile([3]int{8, 8, 2}, []messaging.Port{port})

		connector.EstablishNetwork()
	})
})

type testOwner struct{}

func (testOwner) NotifyRecv(messaging.Port)     {}
func (testOwner) NotifyPortFree(messaging.Port) {}
