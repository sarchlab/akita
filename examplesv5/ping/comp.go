package ping

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v4/sim"
)

type PingReq struct {
	sim.MsgMeta

	SeqID int
}

func (p *PingReq) Meta() *sim.MsgMeta {
	return &p.MsgMeta
}

func (p *PingReq) Clone() sim.Msg {
	cloneMsg := *p
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

func (p *PingReq) GenerateRsp() sim.Rsp {
	rsp := &PingRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.RspTo = p.ID

	return rsp
}

type PingRsp struct {
	sim.MsgMeta

	RspTo string
	SeqID int
}

func (p *PingRsp) Meta() *sim.MsgMeta {
	return &p.MsgMeta
}

func (p *PingRsp) Clone() sim.Msg {
	cloneMsg := *p
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

func (p *PingRsp) GetRspTo() string {
	return p.RspTo
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
		SeqID: c.nextSeqID,
	}

	pingMsg.Src = c.OutPort.AsRemote()
	pingMsg.Dst = evt.Dst

	c.OutPort.Send(pingMsg)

	c.startTime = append(c.startTime, evt.Time())

	c.nextSeqID++
}

func (c *Comp) RspPing(evt RspPingEvent) {
	msg := evt.pingMsg
	rsp := &PingRsp{
		SeqID: msg.SeqID,
	}
	rsp.Src = c.OutPort.AsRemote()
	rsp.Dst = msg.Src

	c.OutPort.Send(rsp)
}

func (c *Comp) NotifyRecv(port sim.Port) {
	c.Lock()
	defer c.Unlock()

	msg := port.RetrieveIncoming()
	switch msg := msg.(type) {
	case *PingReq:
		c.processPingMsg(msg)
	case *PingRsp:
		c.processPingRsp(msg)
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
