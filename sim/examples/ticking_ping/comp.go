package ticking_ping

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
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

type pingTransaction struct {
	req       *PingMsg
	cycleLeft int
}

type Comp struct {
	*sim.TickingComponent

	OutPort sim.Port

	currentTransactions []*pingTransaction
	startTime           []sim.VTimeInSec
	numPingNeedToSend   int
	nextSeqID           int
	pingDst             sim.Port
}

func (c *Comp) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = c.sendRsp(now) || madeProgress
	madeProgress = c.sendPing(now) || madeProgress
	madeProgress = c.countDown() || madeProgress
	madeProgress = c.processInput(now) || madeProgress

	return madeProgress
}

func (c *Comp) processInput(now sim.VTimeInSec) bool {
	msg := c.OutPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *PingMsg:
		c.processingPingMsg(now, msg)
	case *PingRsp:
		c.processingPingRsp(now, msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (c *Comp) processingPingMsg(
	now sim.VTimeInSec,
	ping *PingMsg,
) {
	trans := &pingTransaction{
		req:       ping,
		cycleLeft: 2,
	}
	c.currentTransactions = append(c.currentTransactions, trans)
	c.OutPort.RetrieveIncoming(now)
}

func (c *Comp) processingPingRsp(
	now sim.VTimeInSec,
	msg *PingRsp,
) {
	seqID := msg.SeqID
	startTime := c.startTime[seqID]
	duration := now - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
	c.OutPort.RetrieveIncoming(now)
}

func (c *Comp) countDown() bool {
	madeProgress := false
	for _, trans := range c.currentTransactions {
		if trans.cycleLeft > 0 {
			trans.cycleLeft--
			madeProgress = true
		}
	}
	return madeProgress
}

func (c *Comp) sendRsp(now sim.VTimeInSec) bool {
	if len(c.currentTransactions) == 0 {
		return false
	}

	trans := c.currentTransactions[0]
	if trans.cycleLeft > 0 {
		return false
	}

	rsp := &PingRsp{
		SeqID: trans.req.SeqID,
	}
	rsp.SendTime = now
	rsp.Src = c.OutPort
	rsp.Dst = trans.req.Src

	err := c.OutPort.Send(rsp)
	if err != nil {
		return false
	}

	c.currentTransactions = c.currentTransactions[1:]

	return true
}

func (c *Comp) sendPing(now sim.VTimeInSec) bool {
	if c.numPingNeedToSend == 0 {
		return false
	}

	pingMsg := &PingMsg{
		SeqID: c.nextSeqID,
	}
	pingMsg.Src = c.OutPort
	pingMsg.Dst = c.pingDst
	pingMsg.SendTime = now

	err := c.OutPort.Send(pingMsg)
	if err != nil {
		return false
	}

	c.startTime = append(c.startTime, now)
	c.numPingNeedToSend--
	c.nextSeqID++

	return true
}

func Example_pingWithTicking() {
	engine := sim.NewSerialEngine()
	// agentA := NewTickingPingAgent("AgentA", engine, 1*sim.Hz)
	agentA := MakeBuilder().WithEngine(engine).WithFreq(1 * sim.Hz).Build("AgentA")
	// agentB := NewTickingPingAgent("AgentB", engine, 1*sim.Hz)
	agentB := MakeBuilder().WithEngine(engine).WithFreq(1 * sim.Hz).Build("AgentB")
	conn := directconnection.MakeBuilder().WithEngine(engine).WithFreq(1 * sim.GHz).Build("Conn")

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
