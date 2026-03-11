package ping

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/sim"
)

// PingReq is a ping request message.
type PingReq struct {
	sim.MsgMeta
	SeqID int
}

// PingRsp is a ping response message.
type PingRsp struct {
	sim.MsgMeta
	SeqID int
}

type StartPingEvent struct {
	*sim.EventBase

	Dst sim.RemotePort
}

type RspPingEvent struct {
	*sim.EventBase

	pingMsg *PingReq
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
	pingMsg := &PingReq{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: c.OutPort.AsRemote(),
			Dst: evt.Dst,
		},
		SeqID: c.nextSeqID,
	}

	c.OutPort.Send(pingMsg)

	c.startTime = append(c.startTime, evt.Time())

	c.nextSeqID++
}

func (c *Comp) RspPing(evt RspPingEvent) {
	msg := evt.pingMsg
	rsp := &PingRsp{
		MsgMeta: sim.MsgMeta{
			ID:    sim.GetIDGenerator().Generate(),
			Src:   c.OutPort.AsRemote(),
			Dst:   msg.Src,
			RspTo: msg.ID,
		},
		SeqID: msg.SeqID,
	}

	c.OutPort.Send(rsp)
}

func (c *Comp) NotifyRecv(port sim.Port) {
	c.Lock()
	defer c.Unlock()

	msg := port.RetrieveIncoming()
	switch m := msg.(type) {
	case *PingReq:
		c.processPingMsg(m)
	case *PingRsp:
		c.processPingRsp(m)
	default:
		panic("cannot process msg of type " + reflect.TypeOf(msg).String())
	}
}

func (c *Comp) processPingMsg(msg *PingReq) {
	rspEvent := RspPingEvent{
		EventBase: sim.NewEventBase(c.Engine.CurrentTime()+2, c),
		pingMsg:   msg,
	}
	c.Engine.Schedule(rspEvent)
}

func (c *Comp) processPingRsp(msg *PingRsp) {
	seqID := msg.SeqID
	startTime := c.startTime[seqID]
	now := c.Engine.CurrentTime()
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
}

func (c Comp) NotifyPortFree(_ sim.Port) {
	// Do nothing
}
