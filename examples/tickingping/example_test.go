package tickingping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

func Example() {
	engine := timing.NewSerialEngine()
	registrar := modeling.NewStandaloneRegistrar(engine)

	agentSpec := DefaultSpec()
	agentSpec.Freq = 1 * timing.Hz

	agentA := MakeBuilder().
		WithRegistrar(registrar).
		WithSpec(agentSpec).
		Build("AgentA")
	agentAOut := modeling.MakePortBuilder().
		WithRegistrar(registrar).
		WithComponent(agentA).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Out")
	agentA.AssignPort("Out", agentAOut)

	agentB := MakeBuilder().
		WithRegistrar(registrar).
		WithSpec(agentSpec).
		Build("AgentB")
	agentBOut := modeling.MakePortBuilder().
		WithRegistrar(registrar).
		WithComponent(agentB).
		WithSpec(modeling.PortSpec{BufSize: 16}).
		Build("Out")
	agentB.AssignPort("Out", agentBOut)

	conn := directconnection.
		MakeBuilder().
		WithRegistrar(registrar).
		Build("Conn")

	conn.PlugIn(agentA.GetPortByName("Out"))
	conn.PlugIn(agentB.GetPortByName("Out"))

	state := agentA.State
	state.PingDst = agentB.GetPortByName("Out").AsRemote()
	state.NumPingNeedToSend = 2
	agentA.State = state

	agentA.TickLater()

	err := engine.Run()
	if err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 5000000000000 ps
	// Ping 1, 5000000000000 ps
}
