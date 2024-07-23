package ping

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

type PingReq struct {
	sim.MsgMeta

	SeqID int
}

func (p *PingReq) Meta() *sim.MsgMeta {
	return &p.MsgMeta
}

func (p *PingReq) Clone() sim.Msg {
	return p
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
	return p
}

func (p *PingRsp) GetRspTo() string {
	return p.RspTo
}

type StartPingEvent struct {
	*sim.EventBase
	Dst sim.Port
}

type RspPingEvent struct {
	*sim.EventBase
	pingMsg *PingReq
}

type Comp struct {
	*sim.ComponentBase

	Engine  sim.Engine
	OutPort sim.Port

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

	pingMsg.Src = c.OutPort
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
	rsp.Src = c.OutPort
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
		EventBase: sim.NewEventBase(c.CurrentTime()+2, c),
		pingMsg:   msg,
	}
	c.Engine.Schedule(rspEvent)
}

func (c *Comp) processPingRsp(msg *PingRsp) {
	seqID := msg.SeqID
	startTime := c.startTime[seqID]
	now := c.CurrentTime()
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
}

func (c Comp) NotifyPortFree(_ sim.Port) {
	// Do nothing
}

func Example_pingWithEvents() {
	engine := sim.NewSerialEngine()
	// agentA := NewPingAgent("AgentA", engine)
	agentA := MakeBuilder().WithEngine(engine).Build("AgentA")
	// agentB := NewPingAgent("AgentB", engine)
	agentB := MakeBuilder().WithEngine(engine).Build("AgentB")
	conn := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn")

	conn.PlugIn(agentA.OutPort, 1)
	conn.PlugIn(agentB.OutPort, 1)

	e1 := StartPingEvent{
		EventBase: sim.NewEventBase(1, agentA),
		Dst:       agentB.OutPort,
	}
	e2 := StartPingEvent{
		EventBase: sim.NewEventBase(3, agentA),
		Dst:       agentB.OutPort,
	}
	engine.Schedule(e1)
	engine.Schedule(e2)

	engine.Run()
	// Output:
	// Ping 0, 2.00
	// Ping 1, 2.00
}

func (c *Comp) CurrentTime() sim.VTimeInSec {
	return c.Engine.CurrentTime()
}
