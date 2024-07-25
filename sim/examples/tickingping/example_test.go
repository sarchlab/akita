package tickingping

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

func Example_pingWithTicking() {
	engine := sim.NewSerialEngine()
	// agentA := NewTickingPingAgent("AgentA", engine, 1*sim.Hz)
	agentA := MakeBuilder().WithEngine(engine).WithFreq(1 * sim.Hz).Build("AgentA")
	// agentB := NewTickingPingAgent("AgentB", engine, 1*sim.Hz)
	agentB := MakeBuilder().WithEngine(engine).WithFreq(1 * sim.Hz).Build("AgentB")
	conn := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn")

	conn.PlugIn(agentA.OutPort, 1)
	conn.PlugIn(agentB.OutPort, 1)

	agentA.pingDst = agentB.OutPort
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
