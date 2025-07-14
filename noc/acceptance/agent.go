package acceptance

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
)

// Agent can send and receive request.
type Agent struct {
	*sim.TickingComponent

	test       *Test
	AgentPorts []sim.Port
	MsgsToSend []sim.Msg
	sendBytes  uint64
	recvBytes  uint64
}

// NewAgent creates a new agent.
func NewAgent(
	engine sim.Engine,
	freq sim.Freq,
	name string,
	numPorts int,
	test *Test,
) *Agent {
	a := &Agent{}
	a.test = test
	a.TickingComponent = sim.NewTickingComponent(name, engine, freq, a)

	for i := 0; i < numPorts; i++ {
		p := sim.NewPort(a, 1, 1, fmt.Sprintf("%s.Port%d", name, i))
		a.AgentPorts = append(a.AgentPorts, p)
	}

	return a
}

// Tick tries to receive requests and send requests out.
func (a *Agent) Tick() bool {
	madeProgress := false
	madeProgress = a.send() || madeProgress
	madeProgress = a.recv() || madeProgress

	return madeProgress
}

func (a *Agent) send() bool {
	if len(a.MsgsToSend) == 0 {
		return false
	}

	msg := a.MsgsToSend[0]
	src := msg.Meta().Src

	srcPort := a.findPortByName(src)

	err := srcPort.Send(msg)
	if err == nil {
		a.MsgsToSend = a.MsgsToSend[1:]
		a.sendBytes += uint64(msg.Meta().TrafficBytes)

		return true
	}

	return false
}

func (a *Agent) findPortByName(src sim.RemotePort) sim.Port {
	var srcPort sim.Port

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
		msg := port.RetrieveIncoming()

		if msg != nil {
			a.test.receiveMsg(msg, port)
			a.recvBytes += uint64(msg.Meta().TrafficBytes)

			// fmt.Printf("%.10f, %s, agent received, msg-%s\n",
			// now, a.Name(), msg.Meta().ID)

			madeProgress = true
		}
	}

	return madeProgress
}

// Ports returns the ports of the agent.
func (a *Agent) Ports() []sim.Port {
	return a.AgentPorts
}
