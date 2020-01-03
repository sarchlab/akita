package akita_test

import (
	"fmt"
	"reflect"

	"gitlab.com/akita/akita"
)

type PingMsg struct {
	akita.MsgMeta

	SeqID int
}

func (p *PingMsg) Meta() *akita.MsgMeta {
	return &p.MsgMeta
}

type PingRsp struct {
	akita.MsgMeta

	SeqID int
}

func (p *PingRsp) Meta() *akita.MsgMeta {
	return &p.MsgMeta
}

type StartPingEvent struct {
	*akita.EventBase
	Dst akita.Port
}

type RspPingEvent struct {
	*akita.EventBase
	pingMsg *PingMsg
}

type PingAgent struct {
	*akita.ComponentBase

	Engine  akita.Engine
	OutPort akita.Port

	startTime []akita.VTimeInSec
	nextSeqID int
}

func NewPingAgent(name string, engine akita.Engine) *PingAgent {
	agent := &PingAgent{Engine: engine}
	agent.ComponentBase = akita.NewComponentBase(name)
	agent.OutPort = akita.NewLimitNumMsgPort(agent, 4, name+".OutPort")
	return agent
}

func (p *PingAgent) Handle(e akita.Event) error {
	p.Lock()
	defer p.Unlock()

	switch e := e.(type) {
	case StartPingEvent:
		p.StartPing(e)
	case RspPingEvent:
		p.RspPing(e)
	default:
		panic("cannot handle event of type " + reflect.TypeOf(e).String())
	}
	return nil
}

func (p *PingAgent) StartPing(evt StartPingEvent) {
	pingMsg := &PingMsg{
		SeqID: p.nextSeqID,
	}

	pingMsg.Src = p.OutPort
	pingMsg.Dst = evt.Dst
	pingMsg.SendTime = evt.Time()

	p.OutPort.Send(pingMsg)

	p.startTime = append(p.startTime, evt.Time())

	p.nextSeqID++
}

func (p *PingAgent) RspPing(evt RspPingEvent) {
	msg := evt.pingMsg
	rsp := &PingRsp{
		SeqID: msg.SeqID,
	}
	rsp.SendTime = evt.Time()
	rsp.Src = p.OutPort
	rsp.Dst = msg.Src

	p.OutPort.Send(rsp)
}

func (p *PingAgent) NotifyRecv(now akita.VTimeInSec, port akita.Port) {
	p.Lock()
	defer p.Unlock()

	msg := port.Retrieve(now)
	switch msg := msg.(type) {
	case *PingMsg:
		p.processPingMsg(now, msg)
	case *PingRsp:
		p.processPingRsp(now, msg)
	default:
		panic("cannot process msg of type " + reflect.TypeOf(msg).String())
	}
}

func (p *PingAgent) processPingMsg(now akita.VTimeInSec, msg *PingMsg) {
	rspEvent := RspPingEvent{
		EventBase: akita.NewEventBase(now+2, p),
		pingMsg:   msg,
	}
	p.Engine.Schedule(rspEvent)
}

func (p *PingAgent) processPingRsp(now akita.VTimeInSec, msg *PingRsp) {
	seqID := msg.SeqID
	startTime := p.startTime[seqID]
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
}

func (p PingAgent) NotifyPortFree(now akita.VTimeInSec, port akita.Port) {
	// Do nothing
}

func Example_pingWithEvents() {
	engine := akita.NewSerialEngine()
	agentA := NewPingAgent("AgentA", engine)
	agentB := NewPingAgent("AgentB", engine)
	conn := akita.NewDirectConnection("Conn", engine, 1*akita.GHz)

	conn.PlugIn(agentA.OutPort, 1)
	conn.PlugIn(agentB.OutPort, 1)

	e1 := StartPingEvent{
		EventBase: akita.NewEventBase(1, agentA),
		Dst:       agentB.OutPort,
	}
	e2 := StartPingEvent{
		EventBase: akita.NewEventBase(3, agentA),
		Dst:       agentB.OutPort,
	}
	engine.Schedule(e1)
	engine.Schedule(e2)

	engine.Run()
	// Output:
	// Ping 0, 2.00
	// Ping 1, 2.00
}
