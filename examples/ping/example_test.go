package ping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

func Example_pingWithEvents() {
	engine := timing.NewSerialEngine()
	registrar := modeling.NewStandaloneRegistrar(engine)

	agentA := MakeBuilder().
		WithRegistrar(registrar).
		Build("AgentA")

	agentB := MakeBuilder().
		WithRegistrar(registrar).
		Build("AgentB")

	conn := directconnection.MakeBuilder().
		WithRegistrar(registrar).
		Build("Conn")

	conn.PlugIn(agentA.GetPortByName("Out"))
	conn.PlugIn(agentB.GetPortByName("Out"))

	SchedulePing(agentA, 1, agentB.GetPortByName("Out").AsRemote())
	SchedulePing(agentA, 3, agentB.GetPortByName("Out").AsRemote())

	engine.Run()
	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
