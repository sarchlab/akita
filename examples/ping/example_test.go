package ping

import (
	"github.com/sarchlab/akita/v5/blueprint"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example_pingWithEvents() {
	sim := akitasim.MakeBuilder().Build()

	sim.Add("", blueprint.NewDomain[modeling.None, modeling.None](modeling.None{}, func(_ modeling.None,
		bp *blueprint.Blueprint) {
		buffers := twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
		bp.Add("AgentA", blueprint.Comp[*Comp](DefaultSpec()).Port("Out", buffers))
		bp.Add("AgentB", blueprint.Comp[*Comp](DefaultSpec()).Port("Out", buffers))
		bp.Connect("Conn", timing.GHz, bp.PortRef("AgentA.Out"), bp.PortRef("AgentB.Out"))
	}))
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	agentA := blueprint.Lookup[*Comp](sim, "AgentA")
	agentB := blueprint.Lookup[*Comp](sim, "AgentB")

	SchedulePing(agentA, 1, agentB.Ports.Out.AsRemote())
	SchedulePing(agentA, 3, agentB.Ports.Out.AsRemote())

	if err := sim.Run(); err != nil {
		panic(err)
	}
	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
