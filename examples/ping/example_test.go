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
	agentAOut := modeling.MakePortBuilder().
		WithRegistrar(registrar).
		WithComponent(agentA).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Out")
	agentA.AssignPort("Out", agentAOut)

	agentB := MakeBuilder().
		WithRegistrar(registrar).
		Build("AgentB")
	agentBOut := modeling.MakePortBuilder().
		WithRegistrar(registrar).
		WithComponent(agentB).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Out")
	agentB.AssignPort("Out", agentBOut)

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
