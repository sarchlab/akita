package ping

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/sim"
)

// PingReqPayload is the payload for a ping request message.
type PingReqPayload struct {
	SeqID int
}

// PingRspPayload is the payload for a ping response message.
type PingRspPayload struct {
	SeqID int
}

type StartPingEvent struct {
	*sim.EventBase

	Dst sim.RemotePort
}

type RspPingEvent struct {
	*sim.EventBase

	pingMsg *sim.Msg
}

type Comp struct {
	*sim.ComponentBase

	OutPort sim.Port
	Engine  sim.Engine

	startTime []sim.VTimeInSec
	nextSeqID int
}

func (c *Comp) Handle(e sim.Event) error {
	c.Lock()
	defer c.Unlock()

	switch e := e.(type) {
	case StartPingEvent:
		c.StartPing(e)
	case RspPingEvent:
		c.RspPing(e)
	default:
		panic("cannot handle event of type " + reflect.TypeOf(e).String())
	}

	return nil
}

func (c *Comp) StartPing(evt StartPingEvent) {
	pingMsg := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: c.OutPort.AsRemote(),
			Dst: evt.Dst,
		},
		Payload: &PingReqPayload{
			SeqID: c.nextSeqID,
		},
	}

	c.OutPort.Send(pingMsg)

	c.startTime = append(c.startTime, evt.Time())

	c.nextSeqID++
}

func (c *Comp) RspPing(evt RspPingEvent) {
	msg := evt.pingMsg
	payload := sim.MsgPayload[PingReqPayload](msg)
	rsp := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: c.OutPort.AsRemote(),
			Dst: msg.Src,
		},
		RspTo: msg.ID,
		Payload: &PingRspPayload{
			SeqID: payload.SeqID,
		},
	}

	c.OutPort.Send(rsp)
}

func (c *Comp) NotifyRecv(port sim.Port) {
	c.Lock()
	defer c.Unlock()

	msg := port.RetrieveIncoming()
	switch msg.Payload.(type) {
	case *PingReqPayload:
		c.processPingMsg(msg)
	case *PingRspPayload:
		c.processPingRsp(msg)
	default:
		panic("cannot process msg of type " + reflect.TypeOf(msg.Payload).String())
	}
}

func (c *Comp) processPingMsg(msg *sim.Msg) {
	rspEvent := RspPingEvent{
		EventBase: sim.NewEventBase(c.Engine.CurrentTime()+2, c),
		pingMsg:   msg,
	}
	c.Engine.Schedule(rspEvent)
}

func (c *Comp) processPingRsp(msg *sim.Msg) {
	payload := sim.MsgPayload[PingRspPayload](msg)
	seqID := payload.SeqID
	startTime := c.startTime[seqID]
	now := c.Engine.CurrentTime()
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
}

func (c Comp) NotifyPortFree(_ sim.Port) {
	// Do nothing
}
