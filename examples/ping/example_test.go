package ping_test

import (
	"github.com/sarchlab/akita/v5/examples/ping"
	"github.com/sarchlab/akita/v5/examples/ping/workload"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func Example_manualHardware() {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()

	buffers := twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}
	agentA := modeling.MakeBuilder[*ping.Comp](ping.DefaultSpec()).WithSimulation(s).Build("AgentA")
	agentB := modeling.MakeBuilder[*ping.Comp](ping.DefaultSpec()).WithSimulation(s).Build("AgentB")
	agentA.BindPort("Out", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(buffers).Build())
	agentB.BindPort("Out", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(buffers).Build())
	conn := direct.NewConnection("Conn", s, timing.GHz)
	conn.BindPort(agentA.Ports.Out)
	conn.BindPort(agentB.Ports.Out)

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
