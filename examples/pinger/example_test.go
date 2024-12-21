package ping

import (
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func Example_pingWithEvents() {
	engine := timing.NewSerialEngine()
	sim := simulation.NewSimulation()
	sim.RegisterEngine(engine)

	agentA := Builder{}.WithSimulation(sim).Build("AgentA")
	agentB := Builder{}.WithSimulation(sim).Build("AgentB")

	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build("Conn")

	conn.PlugIn(agentA.OutPort)
	conn.PlugIn(agentB.OutPort)

	e1 := StartPingEvent{
		EventBase: timing.NewEventBase(1, agentA),
		Dst:       agentB.OutPort.AsRemote(),
	}
	e2 := StartPingEvent{
		EventBase: timing.NewEventBase(3, agentA),
		Dst:       agentB.OutPort.AsRemote(),
	}

	engine.Schedule(e1)
	engine.Schedule(e2)

	engine.Run()
	// Output:
	// Ping 0, 2.00
	// Ping 1, 2.00
}
