package ping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

func Example_pingWithEvents() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	agentA := Definition.Builder().
		WithSimulation(sim).
		WithPorts(Ports{Out: messaging.NewPort(nil, 16, 16, "AgentA.Out")}).
		Build("AgentA")

	agentB := Definition.Builder().
		WithSimulation(sim).
		WithPorts(Ports{Out: messaging.NewPort(nil, 16, 16, "AgentB.Out")}).
		Build("AgentB")

	conn := directconnection.MakeBuilder().
		WithSimulation(sim).
		Build("Conn")

	conn.PlugIn(agentA.Ports.Out)
	conn.PlugIn(agentB.Ports.Out)

	SchedulePing(agentA, 1, agentB.Ports.Out.AsRemote())
	SchedulePing(agentA, 3, agentB.Ports.Out.AsRemote())

	if err := engine.Run(); err != nil {
		panic(err)
	}
	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
