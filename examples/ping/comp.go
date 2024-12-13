package ping

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type PingReq struct {
	modeling.MsgMeta

	SeqID int
}

func (p PingReq) Meta() modeling.MsgMeta {
	return p.MsgMeta
}

func (p PingReq) Clone() modeling.Msg {
	cloneMsg := p
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

func (p PingReq) GenerateRsp() PingRsp {
	rsp := PingRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: p.Dst,
			Dst: p.Src,
		},
		SeqID: p.SeqID,
	}

	return rsp
}

type PingRsp struct {
	modeling.MsgMeta

	SeqID int
}

func (p PingRsp) Meta() modeling.MsgMeta {
	return p.MsgMeta
}

func (p PingRsp) Clone() modeling.Msg {
	cloneMsg := p
	cloneMsg.ID = id.Generate()

	return cloneMsg
}

type StartPingEvent struct {
	*timing.EventBase
	Dst modeling.RemotePort
}

type RspPingEvent struct {
	*timing.EventBase
	pingMsg PingReq
}

type Comp struct {
	*modeling.ComponentBase

	OutPort modeling.Port
	Engine  timing.Engine

	startTime []timing.VTimeInSec
	nextSeqID int
}

func (c *Comp) Handle(e timing.Event) error {
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
	pingMsg := PingReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
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
	rsp := msg.GenerateRsp()

	c.OutPort.Send(rsp)
}

func (c *Comp) NotifyRecv(port modeling.Port) {
	c.Lock()
	defer c.Unlock()

	msg := port.RetrieveIncoming()
	switch msg := msg.(type) {
	case PingReq:
		c.processPingMsg(msg)
	case PingRsp:
		c.processPingRsp(msg)
	default:
		panic("cannot process msg of type " + reflect.TypeOf(msg).String())
	}
}

func (c *Comp) processPingMsg(msg PingReq) {
	rspEvent := RspPingEvent{
		EventBase: timing.NewEventBase(c.Engine.Now()+2, c),
		pingMsg:   msg,
	}
	c.Engine.Schedule(rspEvent)
}

func (c *Comp) processPingRsp(msg PingRsp) {
	seqID := msg.SeqID
	startTime := c.startTime[seqID]
	now := c.Engine.Now()
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
}

func (c Comp) NotifyPortFree(_ modeling.Port) {
	// Do nothing
}
