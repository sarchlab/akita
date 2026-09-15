package ping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

func Example_pingWithEvents() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	agentA := MakeBuilder().
		WithSimulation(sim).
		Build("AgentA")
	agentAOut := modeling.MakePortBuilder().
		WithSimulation(sim).
		WithComponent(agentA).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Out")
	agentA.AssignPort("Out", agentAOut)

	agentB := MakeBuilder().
		WithSimulation(sim).
		Build("AgentB")
	agentBOut := modeling.MakePortBuilder().
		WithSimulation(sim).
		WithComponent(agentB).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Out")
	agentB.AssignPort("Out", agentBOut)

	conn := directconnection.MakeBuilder().
		WithSimulation(sim).
		Build("Conn")

	conn.PlugIn(agentA.GetPortByName("Out"))
	conn.PlugIn(agentB.GetPortByName("Out"))

	SchedulePing(agentA, 1, agentB.GetPortByName("Out").AsRemote())
	SchedulePing(agentA, 3, agentB.GetPortByName("Out").AsRemote())

	if err := engine.Run(); err != nil {
		panic(err)
	}
	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
