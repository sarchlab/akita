package tickingping

import (
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	// Choose buffer sizes; binding below assigns each port its name.
	outA := twowaybuffered.NewPort(16, 16)
	outB := twowaybuffered.NewPort(16, 16)

	specA := Definition.DefaultSpec
	specA.Freq = 1 * timing.Hz
	specA.PingDst = "AgentB.Out"
	specA.NumPings = 2

	specB := Definition.DefaultSpec
	specB.Freq = 1 * timing.Hz

	agentA := Definition.Builder().
		WithSimulation(sim).
		WithSpec(specA).
		Build("AgentA")

	agentA.BindPort("Out", outA)

	agentB := Definition.Builder().
		WithSimulation(sim).
		WithSpec(specB).
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

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 5000000000000 ps
	// Ping 1, 5000000000000 ps
}
