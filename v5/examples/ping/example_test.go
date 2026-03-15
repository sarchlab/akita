package ping

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/noc/directconnection"
)

func Example_pingWithEvents() {
	engine := sim.NewSerialEngine()

	agentA := MakeBuilder().
		WithEngine(engine).
		WithOutPort(sim.NewPort(nil, 4, 4, "AgentA.OutPort")).
		Build("AgentA")

	agentB := MakeBuilder().
		WithEngine(engine).
		WithOutPort(sim.NewPort(nil, 4, 4, "AgentB.OutPort")).
		Build("AgentB")

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	conn.PlugIn(agentA.GetSpec().OutPort)
	conn.PlugIn(agentB.GetSpec().OutPort)

	SchedulePing(agentA, 1, agentB.GetSpec().OutPort.AsRemote())
	SchedulePing(agentA, 3, agentB.GetSpec().OutPort.AsRemote())

	engine.Run()
	// Output:
	// Ping 0, 2.00
	// Ping 1, 2.00
}
