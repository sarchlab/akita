package acceptance

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	// Agent can send and receive request.
	"github.com/sarchlab/akita/v5/messaging"
)

type Agent struct {
	*modeling.TickingComponent

	test       *Test
	AgentPorts []messaging.Port
	MsgsToSend []*TrafficMsg
	sendBytes  uint64
	recvBytes  uint64
}

// NewAgent creates a new agent.
func NewAgent(
	engine timing.EventScheduler,
	freq timing.Freq,
	name string,
	ports []messaging.Port,
	test *Test,
) *Agent {
	a := &Agent{}
	a.test = test
	a.TickingComponent = modeling.NewTickingComponent(name, engine, freq, a)

	for _, p := range ports {
		p.SetComponent(a)
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
	src := msg.Src

	srcPort := a.findPortByName(src)

	err := srcPort.Send(msg)
	if err == nil {
		a.MsgsToSend = a.MsgsToSend[1:]
		a.sendBytes += uint64(msg.TrafficBytes)

		return true
	}

	return false
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
		msgI := port.RetrieveIncoming()

		if msgI != nil {
			meta := msgI.Meta()
			a.test.receiveMsgMeta(meta, port)
			a.recvBytes += uint64(meta.TrafficBytes)

			madeProgress = true
		}
	}

	return madeProgress
}

// Ports returns the ports of the agent.
func (a *Agent) Ports() []messaging.Port {
	return a.AgentPorts
}
