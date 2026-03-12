// LEGACY: This package uses the old event-driven model (sim.ComponentBase +
// Handle/NotifyRecv). For the canonical tick-based modeling.Component pattern,
// see examples/tickingping instead.

package ping

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/directconnection"
)

func Example_pingWithEvents() {
	engine := sim.NewSerialEngine()
	// agentA := NewPingAgent("AgentA", engine)
	agentA := MakeBuilder().
		WithEngine(engine).
		WithOutPort(sim.NewPort(nil, 4, 4, "AgentA.OutPort")).
		Build("AgentA")
	// agentB := NewPingAgent("AgentB", engine)
	agentB := MakeBuilder().
		WithEngine(engine).
		WithOutPort(sim.NewPort(nil, 4, 4, "AgentB.OutPort")).
		Build("AgentB")
	conn := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Conn")

	conn.PlugIn(agentA.OutPort)
	conn.PlugIn(agentB.OutPort)

	e1 := StartPingEvent{
		EventBase: sim.NewEventBase(1, agentA),
		Dst:       agentB.OutPort.AsRemote(),
	}
	e2 := StartPingEvent{
		EventBase: sim.NewEventBase(3, agentA),
		Dst:       agentB.OutPort.AsRemote(),
	}

	engine.Schedule(e1)
	engine.Schedule(e2)

	engine.Run()
	// Output:
	// Ping 0, 2.00
	// Ping 1, 2.00
}
