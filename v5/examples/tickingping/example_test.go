package tickingping

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/directconnection"
)

func Example() {
	engine := sim.NewSerialEngine()
	agentA := MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.Hz).
		WithOutPort(sim.NewPort(nil, 4, 4, "AgentA.OutPort")).
		Build("AgentA")
	agentB := MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.Hz).
		WithOutPort(sim.NewPort(nil, 4, 4, "AgentB.OutPort")).
		Build("AgentB")
	conn := directconnection.
		MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	conn.PlugIn(agentA.GetPortByName("Out"))
	conn.PlugIn(agentB.GetPortByName("Out"))

	state := agentA.GetState()
	state.PingDst = agentB.GetPortByName("Out").AsRemote()
	state.NumPingNeedToSend = 2
	agentA.SetState(state)

	agentA.TickLater()

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 5.00
	// Ping 1, 5.00
}
