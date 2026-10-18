package mesh_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	"github.com/sarchlab/akita/v5/noc/mesh"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestConnector(t *testing.T) {
	var (
		engine    timing.Engine
		sim       timing.Simulation
		connector *mesh.Connector
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)
		connector = mesh.MakeBuilder().WithSimulation(sim).Build("Network")
	}

	t.Run("should be able to connect ports outside current capacity", func(t *testing.T) {
		setup1(t)

		port := twowaybuffered.NewPort(1, 1)
		port.BindOwner(testOwner{}, "Device.Port")

		// 8,8,2 is the default capacity
		connector.AddTile([3]int{8, 8, 2}, []messaging.Port{port})
		require.NoError(t, sim.Initialize())
	})
}

type testOwner struct{}

func (testOwner) NotifyRecv(messaging.Port)     {}
func (testOwner) NotifyPortFree(messaging.Port) {}

func (o testOwner) Name() string                           { return "TestOwner" }
func (o testOwner) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
