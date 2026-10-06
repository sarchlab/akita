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

	// Create the ports first, so AgentA's Spec can name AgentB's port.
	outA := twowaybuffered.NewPort("AgentA.Out", 16, 16)
	outB := twowaybuffered.NewPort("AgentB.Out", 16, 16)

	specA := Definition.DefaultSpec
	specA.Freq = 1 * timing.Hz
	specA.PingDst = outB.AsRemote()
	specA.NumPings = 2

	specB := Definition.DefaultSpec
	specB.Freq = 1 * timing.Hz

	agentA := Definition.Builder().
		WithSimulation(sim).
		WithSpec(specA).
		WithPorts(Ports{Out: outA}).
		Build("AgentA")

	agentB := Definition.Builder().
		WithSimulation(sim).
		WithSpec(specB).
		WithPorts(Ports{Out: outB}).
		Build("AgentB")

	conn := direct.Definition.Builder().
		WithSimulation(sim).
		Build("Conn")

	conn.PlugIn(agentA.Ports.Out)
	conn.PlugIn(agentB.Ports.Out)

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
