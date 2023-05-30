package sim_test

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
)

type PingMsg struct {
	sim.MsgMeta

	SeqID int
}

func (p *PingMsg) Meta() *sim.MsgMeta {
	return &p.MsgMeta
}

type PingRsp struct {
	sim.MsgMeta

	SeqID int
}

func (p *PingRsp) Meta() *sim.MsgMeta {
	return &p.MsgMeta
}

type StartPingEvent struct {
	*sim.EventBase
	Dst sim.Port
}

type RspPingEvent struct {
	*sim.EventBase
	pingMsg *PingMsg
}

type PingAgent struct {
	*sim.ComponentBase

	Engine  sim.Engine
	OutPort sim.Port

	startTime []sim.VTimeInSec
	nextSeqID int
}

func NewPingAgent(name string, engine sim.Engine) *PingAgent {
	agent := &PingAgent{Engine: engine}
	agent.ComponentBase = sim.NewComponentBase(name)
	agent.OutPort = sim.NewLimitNumMsgPort(agent, 4, name+".OutPort")
	return agent
}

func (p *PingAgent) Handle(e sim.Event) error {
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

func (p *PingAgent) NotifyRecv(now sim.VTimeInSec, port sim.Port) {
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

func (p *PingAgent) processPingMsg(now sim.VTimeInSec, msg *PingMsg) {
	rspEvent := RspPingEvent{
		EventBase: sim.NewEventBase(now+2, p),
		pingMsg:   msg,
	}
	p.Engine.Schedule(rspEvent)
}

func (p *PingAgent) processPingRsp(now sim.VTimeInSec, msg *PingRsp) {
	seqID := msg.SeqID
	startTime := p.startTime[seqID]
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
}

func (p PingAgent) NotifyPortFree(_ sim.VTimeInSec, _ sim.Port) {
	// Do nothing
}

func Example_pingWithEvents() {
	engine := sim.NewSerialEngine()
	agentA := NewPingAgent("AgentA", engine)
	agentB := NewPingAgent("AgentB", engine)
	conn := sim.NewDirectConnection("Conn", engine, 1*sim.GHz)

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
