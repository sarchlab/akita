package acceptance

import (
	"fmt"

	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/internal/tickscheduler"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Agent is a traffic generator for network tests. It sends the messages in
// MsgsToSend out of its ports and reports every message it receives to the
// test. It is a test double, not a modeled component, so it is written
// directly on a tickscheduler.Scheduler.
type Agent struct {
	*tickscheduler.Scheduler
	hooking.HookableBase

	name       string
	simulation modeling.Simulation
	test       *Test
	AgentPorts []messaging.Port
	MsgsToSend []messaging.Msg
	sendBytes  uint64
	recvBytes  uint64
}

// NewAgent creates a new agent and registers it (and its ports) with the
// simulation, so the agent appears in the simulation's topology and trace.
func NewAgent(
	sim modeling.Simulation,
	freq timing.Freq,
	name string,
	ports []messaging.Port,
	test *Test,
) *Agent {
	a := &Agent{
		Scheduler:  tickscheduler.NewScheduler(name, sim, freq),
		name:       name,
		simulation: sim,
		test:       test,
	}

	sim.Engine().RegisterHandler(name, a)

	sim.RegisterComponent(a)

	for i, p := range ports {
		a.BindPort(fmt.Sprintf("Port[%d]", i), p)
	}

	return a
}

// Name returns the agent's name.
func (a *Agent) Name() string {
	return a.name
}

// Handle tries to receive and send messages on a tick, and ticks again if it
// made progress.
func (a *Agent) Handle(_ timing.Event) {
	madeProgress := false
	madeProgress = a.send() || madeProgress
	madeProgress = a.recv() || madeProgress

	if madeProgress {
		a.TickLater()
	}
}

// NotifyRecv wakes the agent when a port receives a message.
func (a *Agent) NotifyRecv(_ messaging.Port) {
	a.TickLater()
}

// NotifyPortFree wakes the agent when a port can send again.
func (a *Agent) NotifyPortFree(_ messaging.Port) {
	a.TickLater()
}

func (a *Agent) send() bool {
	if len(a.MsgsToSend) == 0 {
		return false
	}

	msg := a.MsgsToSend[0]
	src := msg.Src

	srcPort := a.findPortByName(src)

	if !srcPort.CanSend() {
		return false
	}

	srcPort.Send(msg)
	a.MsgsToSend = a.MsgsToSend[1:]
	a.sendBytes += uint64(msg.TrafficBytes)

	return true
}

func (a *Agent) findPortByName(src messaging.RemotePort) messaging.Port {
	var srcPort messaging.Port

	for _, port := range a.AgentPorts {
		if port.AsRemote() == src {
			srcPort = port
			break
		}
	}

	if srcPort == nil {
		panic(fmt.Sprintf("src port not found for %s", src))
	}

	return srcPort
}

func (a *Agent) recv() bool {
	madeProgress := false

	for _, port := range a.AgentPorts {
		msgI, ok := port.RetrieveIncoming()

		if ok {
			meta := msgI
			a.test.receiveMsg(meta, port)
			a.recvBytes += uint64(meta.TrafficBytes)

			madeProgress = true
		}
	}

	return madeProgress
}

// BindPort attaches and registers an agent port during network assembly.
func (a *Agent) BindPort(slot string, p messaging.Port) {
	assembly.RequireSetup(a.simulation)
	name := a.Name() + "." + slot
	assembly.SetupOf(a.simulation).RequireNameAvailable(name)
	assembly.RequireSameSimulation(a.simulation, p)
	p.BindOwner(a, name)
	a.simulation.RegisterPort(p)
	a.AgentPorts = append(a.AgentPorts, p)
}

// NewID allocates a message ID from the owning simulation.
func (a *Agent) NewID() uint64 { return a.simulation.NewID() }
