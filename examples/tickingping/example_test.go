package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

func Example() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	// Create the ports first, so AgentA's Spec can name AgentB's port.
	outA := messaging.NewPort(nil, 16, 16, "AgentA.Out")
	outB := messaging.NewPort(nil, 16, 16, "AgentB.Out")

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

	conn := directconnection.
		MakeBuilder().
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
