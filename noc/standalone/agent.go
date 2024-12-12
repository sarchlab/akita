package standalone

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// TrafficMsg is a type of msg that only used in standalone network test.
// It has a byte size, but we do not care about the information it carries.
type TrafficMsg struct {
	modeling.MsgMeta
}

// Meta returns the meta data of the message.
func (m TrafficMsg) Meta() modeling.MsgMeta {
	return m.MsgMeta
}

// Clone returns cloned TrafficMsg
func (m TrafficMsg) Clone() modeling.Msg {
	cloneMsg := m
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

// StartSendEvent is an event that triggers an agent to send a message.
type StartSendEvent struct {
	*timing.EventBase
	Msg TrafficMsg
}

// NewStartSendEvent creates a new StartSendEvent.
func NewStartSendEvent(
	time timing.VTimeInSec,
	src, dst *Agent,
	byteSize int,
	trafficClass int,
) *StartSendEvent {
	e := new(StartSendEvent)
	e.EventBase = timing.NewEventBase(time, src)
	e.Msg = TrafficMsg{
		MsgMeta: modeling.MsgMeta{
			Src:          src.ToOut.AsRemote(),
			Dst:          dst.ToOut.AsRemote(),
			TrafficBytes: byteSize,
			TrafficClass: trafficClass,
		},
	}

	return e
}

// Agent is a component that connects the network. It can send and receive
// msg to/ from the network.
type Agent struct {
	*modeling.TickingComponent

	ToOut modeling.Port

	Buffer []TrafficMsg
}

// NotifyRecv notifies that a port has received a message.
func (a *Agent) NotifyRecv(port modeling.Port) {
	a.ToOut.RetrieveIncoming()
	a.TickLater()
}

// Handle defines how an agent handles events.
func (a *Agent) Handle(e timing.Event) error {
	switch e := e.(type) {
	case *StartSendEvent:
		a.handleStartSendEvent(e)
	case timing.TickEvent:
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
func NewAgent(name string, engine timing.Engine) *Agent {
	a := new(Agent)
	a.TickingComponent = modeling.NewTickingComponent(
		name, engine, 1*timing.GHz, a)

	a.ToOut = modeling.NewPort(a, 4, 4, name+".ToOut")

	return a
}
