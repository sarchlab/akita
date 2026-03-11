package standalone

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/sim"
)

// TrafficMsg is a traffic message used in standalone network tests.
type TrafficMsg struct {
	sim.MsgMeta
}

// NewTrafficMsg creates a new traffic message
func NewTrafficMsg(src, dst sim.RemotePort, byteSize int) *TrafficMsg {
	return &TrafficMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          src,
			Dst:          dst,
			TrafficBytes: byteSize,
			TrafficClass: reflect.TypeOf(TrafficMsg{}).String(),
		},
	}
}

// StartSendEvent is an event that triggers an agent to send a message.
type StartSendEvent struct {
	*sim.EventBase

	Msg *TrafficMsg
}

// NewStartSendEvent creates a new StartSendEvent.
func NewStartSendEvent(
	time sim.VTimeInSec,
	src, dst *Agent,
	byteSize int,
) *StartSendEvent {
	e := new(StartSendEvent)
	e.EventBase = sim.NewEventBase(time, src)
	e.Msg = NewTrafficMsg(src.ToOut.AsRemote(), dst.ToOut.AsRemote(), byteSize)

	return e
}

// Agent is a component that connects the network. It can send and receive
// msg to/ from the network.
type Agent struct {
	*sim.TickingComponent

	ToOut sim.Port

	Buffer []*TrafficMsg
}

// NotifyRecv notifies that a port has received a message.
func (a *Agent) NotifyRecv(port sim.Port) {
	a.ToOut.RetrieveIncoming()
	a.TickLater()
}

// Handle defines how an agent handles events.
func (a *Agent) Handle(e sim.Event) error {
	switch e := e.(type) {
	case *StartSendEvent:
		a.handleStartSendEvent(e)
	case sim.TickEvent:
		err := a.TickingComponent.Handle(e)
		if err != nil {
			return err
		}
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
	}

	return nil
}

func (a *Agent) handleStartSendEvent(e *StartSendEvent) {
	a.Buffer = append(a.Buffer, e.Msg)
	a.TickLater()
}

// Tick attempts to send a message out.
func (a *Agent) Tick() bool {
	return a.sendDataOut()
}

func (a *Agent) sendDataOut() bool {
	if len(a.Buffer) == 0 {
		return false
	}

	msg := a.Buffer[0]

	err := a.ToOut.Send(msg)
	if err == nil {
		a.Buffer = a.Buffer[1:]
		return true
	}

	return false
}

// NewAgent creates a new agent.
func NewAgent(name string, engine sim.Engine, toOut sim.Port) *Agent {
	a := new(Agent)
	a.TickingComponent = sim.NewTickingComponent(name, engine, 1*sim.GHz, a)

	a.ToOut = toOut
	a.ToOut.SetComponent(a)

	return a
}
