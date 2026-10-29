package ping_test

import (
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/examples/ping"
	"github.com/sarchlab/akita/v5/examples/ping/workload"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example_blueprintHardware() {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()

	s.Add("", blueprint.NewDomain[modeling.None, modeling.None](modeling.None{},
		func(_ modeling.None, bp *blueprint.Blueprint) {
			buffers := twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
			bp.Add("AgentA", blueprint.Comp[*ping.Comp](ping.DefaultSpec()).Port("Out", buffers))
			bp.Add("AgentB", blueprint.Comp[*ping.Comp](ping.DefaultSpec()).Port("Out", buffers))
			bp.Connect("Conn", timing.GHz, bp.PortRef("AgentA.Out"), bp.PortRef("AgentB.Out"))
		}))

	if err := s.Initialize(); err != nil {
		panic(err)
	}
	workload.New(workload.DefaultSpec()).Execute(s)
	if err := s.Run(); err != nil {
		panic(err)
	}

	// Output:
	// Ping 0, 2000000000999 ps
	// Ping 1, 2000000000997 ps
}
