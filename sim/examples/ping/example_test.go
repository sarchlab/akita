package ping

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

func Example_pingWithEvents() {
	engine := sim.NewSerialEngine()
	// agentA := NewPingAgent("AgentA", engine)
	agentA := MakeBuilder().WithEngine(engine).Build("AgentA")
	// agentB := NewPingAgent("AgentB", engine)
	agentB := MakeBuilder().WithEngine(engine).Build("AgentB")
	conn := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn")

	conn.PlugIn(agentA.OutPort, 1)
	conn.PlugIn(agentB.OutPort, 1)

	e1 := StartPingEvent{
		EventBase: sim.NewEventBase(1, agentA),
		Dst:       agentB.OutPort,
	}
	e2 := StartPingEvent{
		EventBase: sim.NewEventBase(3, agentA),
		Dst:       agentB.OutPort,
	}
	engine.Schedule(e1)
	engine.Schedule(e2)

	engine.Run()
	// Output:
	// Ping 0, 2.00
	// Ping 1, 2.00
}
