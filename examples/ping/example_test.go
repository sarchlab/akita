package ping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

func Example_pingWithEvents() {
	engine := timing.NewSerialEngine()

	agentA := MakeBuilder().
		WithEngine(engine).
		WithOutPort(messaging.NewPort(nil, 4, 4, "AgentA.OutPort")).
		Build("AgentA")

	agentB := MakeBuilder().
		WithEngine(engine).
		WithOutPort(messaging.NewPort(nil, 4, 4, "AgentB.OutPort")).
		Build("AgentB")

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("Conn")

	conn.PlugIn(agentA.Spec.OutPort)
	conn.PlugIn(agentB.Spec.OutPort)

	SchedulePing(agentA, 1, agentB.Spec.OutPort.AsRemote())
	SchedulePing(agentA, 3, agentB.Spec.OutPort.AsRemote())

	engine.Run()
	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
