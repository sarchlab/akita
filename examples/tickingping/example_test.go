package tickingping

import (
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example() {
	sim := akitasim.MakeBuilder().Build()

	// Choose buffer sizes; binding below assigns each port its name.
	outA := twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build()
	outB := twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build()

	specA := DefaultSpec()
	specA.Freq = 1 * timing.Hz
	specA.PingDst = "AgentB.Out"
	specA.NumPings = 2

	specB := DefaultSpec()
	specB.Freq = 1 * timing.Hz

	agentA := modeling.MakeBuilder[*Comp](specA).
		WithSimulation(sim).
		Build("AgentA")

	agentA.BindPort("Out", outA)

	agentB := modeling.MakeBuilder[*Comp](specB).
		WithSimulation(sim).
		Build("AgentB")

	agentB.BindPort("Out", outB)

	conn := direct.NewConnection("Conn", sim, timing.GHz)

	conn.BindPort(agentA.Ports.Out)
	conn.BindPort(agentB.Ports.Out)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	// AgentA sends pings on its own, so start it; AgentB wakes when a ping
	// arrives.
	agentA.TickLater()

	err := sim.Run()
	if err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 5000000000000 ps
	// Ping 1, 5000000000000 ps
}
