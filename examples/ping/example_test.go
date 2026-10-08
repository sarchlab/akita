package ping

import (
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example_pingWithEvents() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	agentA := Definition.Builder().
		WithSimulation(sim).
		Build("AgentA")

	agentA.BindPort("Out", twowaybuffered.NewPort(16, 16))

	agentB := Definition.Builder().
		WithSimulation(sim).
		Build("AgentB")

	agentB.BindPort("Out", twowaybuffered.NewPort(16, 16))

	conn := direct.NewConnection("Conn", sim, timing.GHz)

	conn.BindPort(agentA.Ports.Out)
	conn.BindPort(agentB.Ports.Out)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	SchedulePing(agentA, 1, agentB.Ports.Out.AsRemote())
	SchedulePing(agentA, 3, agentB.Ports.Out.AsRemote())

	if err := engine.Run(); err != nil {
		panic(err)
	}
	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
