package tickingping

import (
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func Example() {
	engine := timing.NewSerialEngine()

	agentA := MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * timing.Hz).
		Build("AgentA")
	agentB := MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * timing.Hz).
		Build("AgentB")
	conn := directconnection.
		MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("Conn")

	conn.PlugIn(agentA.OutPort)
	conn.PlugIn(agentB.OutPort)

	agentA.pingDst = agentB.OutPort.AsRemote()
	agentA.numPingNeedToSend = 2

	agentA.TickLater()

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 5.00
	// Ping 1, 5.00
}
