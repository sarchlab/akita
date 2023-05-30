package sim_test

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
)

type pingTransaction struct {
	req       *PingMsg
	cycleLeft int
}

type TickingPingAgent struct {
	*sim.TickingComponent

	OutPort sim.Port

	currentTransactions []*pingTransaction
	startTime           []sim.VTimeInSec
	numPingNeedToSend   int
	nextSeqID           int
	pingDst             sim.Port
}

func NewTickingPingAgent(
	name string,
	engine sim.Engine,
	freq sim.Freq,
) *TickingPingAgent {
	a := &TickingPingAgent{}
	a.TickingComponent = sim.NewTickingComponent(name, engine, freq, a)
	a.OutPort = sim.NewLimitNumMsgPort(a, 4, a.Name()+".OutPort")
	return a
}

func (a *TickingPingAgent) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = a.sendRsp(now) || madeProgress
	madeProgress = a.sendPing(now) || madeProgress
	madeProgress = a.countDown() || madeProgress
	madeProgress = a.processInput(now) || madeProgress

	return madeProgress
}

func (a *TickingPingAgent) processInput(now sim.VTimeInSec) bool {
	msg := a.OutPort.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *PingMsg:
		a.processingPingMsg(now, msg)
	case *PingRsp:
		a.processingPingRsp(now, msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (a *TickingPingAgent) processingPingMsg(
	now sim.VTimeInSec,
	ping *PingMsg,
) {
	trans := &pingTransaction{
		req:       ping,
		cycleLeft: 2,
	}
	a.currentTransactions = append(a.currentTransactions, trans)
	a.OutPort.Retrieve(now)
}

func (a *TickingPingAgent) processingPingRsp(
	now sim.VTimeInSec,
	msg *PingRsp,
) {
	seqID := msg.SeqID
	startTime := a.startTime[seqID]
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
	a.OutPort.Retrieve(now)
}

func (a *TickingPingAgent) countDown() bool {
	madeProgress := false
	for _, trans := range a.currentTransactions {
		if trans.cycleLeft > 0 {
			trans.cycleLeft--
			madeProgress = true
		}
	}
	return madeProgress
}

func (a *TickingPingAgent) sendRsp(now sim.VTimeInSec) bool {
	if len(a.currentTransactions) == 0 {
		return false
	}

	trans := a.currentTransactions[0]
	if trans.cycleLeft > 0 {
		return false
	}

	rsp := &PingRsp{
		SeqID: trans.req.SeqID,
	}
	rsp.SendTime = now
	rsp.Src = a.OutPort
	rsp.Dst = trans.req.Src

	err := a.OutPort.Send(rsp)
	if err != nil {
		return false
	}

	a.currentTransactions = a.currentTransactions[1:]

	return true
}

func (a *TickingPingAgent) sendPing(now sim.VTimeInSec) bool {
	if a.numPingNeedToSend == 0 {
		return false
	}

	pingMsg := &PingMsg{
		SeqID: a.nextSeqID,
	}
	pingMsg.Src = a.OutPort
	pingMsg.Dst = a.pingDst
	pingMsg.SendTime = now

	err := a.OutPort.Send(pingMsg)
	if err != nil {
		return false
	}

	a.startTime = append(a.startTime, now)
	a.numPingNeedToSend--
	a.nextSeqID++

	return true
}

func Example_pingWithTicking() {
	engine := sim.NewSerialEngine()
	agentA := NewTickingPingAgent("AgentA", engine, 1*sim.Hz)
	agentB := NewTickingPingAgent("AgentB", engine, 1*sim.Hz)
	conn := sim.NewDirectConnection("Conn", engine, 1*sim.GHz)

	conn.PlugIn(agentA.OutPort, 1)
	conn.PlugIn(agentB.OutPort, 1)

	agentA.pingDst = agentB.OutPort
	agentA.numPingNeedToSend = 2

	agentA.TickLater(0)

	engine.Run()
	// Output:
	// Ping 0, 5.00
	// Ping 1, 5.00
}
