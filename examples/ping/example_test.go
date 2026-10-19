package ping

import (
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example_pingWithEvents() {
	sim := akitasim.MakeBuilder().Build()

	agentA := modeling.MakeBuilder[*Comp](DefaultSpec()).
		WithSimulation(sim).
		Build("AgentA")

	agentA.BindPort("Out", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())

	agentB := modeling.MakeBuilder[*Comp](DefaultSpec()).
		WithSimulation(sim).
		Build("AgentB")

	agentB.BindPort("Out", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())

	conn := direct.NewConnection("Conn", sim, timing.GHz)

	conn.BindPort(agentA.Ports.Out)
	conn.BindPort(agentB.Ports.Out)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	SchedulePing(agentA, 1, agentB.Ports.Out.AsRemote())
	SchedulePing(agentA, 3, agentB.Ports.Out.AsRemote())

	if err := sim.Run(); err != nil {
		panic(err)
	}
	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
