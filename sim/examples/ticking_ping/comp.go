package ticking_ping

import (
	"fmt"
	"strconv"

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

func (p *PingMsg) Clone() sim.Msg {
	cloneMsg := *p
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

func (p *PingRsp) GenerateRsp() sim.Rsp {
	rsp := &PingRsp{}

	return rsp
}

type PingRsp struct {
	sim.MsgMeta

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
	return strconv.Itoa(p.SeqID)
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

func (c *Comp) Tick() bool {
	madeProgress := false

	madeProgress = c.sendRsp() || madeProgress
	madeProgress = c.sendPing() || madeProgress
	madeProgress = c.countDown() || madeProgress
	madeProgress = c.processInput() || madeProgress

	return madeProgress
}

func (c *Comp) processInput() bool {
	msg := c.OutPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *PingMsg:
		c.processingPingMsg(msg)
	case *PingRsp:
		c.processingPingRsp(msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (c *Comp) processingPingMsg(
	ping *PingMsg,
) {
	trans := &pingTransaction{
		req:       ping,
		cycleLeft: 2,
	}
	c.currentTransactions = append(c.currentTransactions, trans)
	c.OutPort.RetrieveIncoming()
}

func (c *Comp) processingPingRsp(
	msg *PingRsp,
) {
	seqID := msg.SeqID
	startTime := c.startTime[seqID]
	currentTime := c.CurrentTime()
	duration := currentTime - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
	c.OutPort.RetrieveIncoming()
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

func (c *Comp) sendRsp() bool {
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
	rsp.Src = c.OutPort
	rsp.Dst = trans.req.Src

	err := c.OutPort.Send(rsp)
	if err != nil {
		return false
	}

	c.currentTransactions = c.currentTransactions[1:]

	return true
}

func (c *Comp) sendPing() bool {
	if c.numPingNeedToSend == 0 {
		return false
	}

	pingMsg := &PingMsg{
		SeqID: c.nextSeqID,
	}
	pingMsg.Src = c.OutPort
	pingMsg.Dst = c.pingDst

	err := c.OutPort.Send(pingMsg)
	if err != nil {
		return false
	}

	c.startTime = append(c.startTime, c.CurrentTime())
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

	agentA.TickLater()

	engine.Run()
	// Output:
	// Ping 0, 5.00
	// Ping 1, 5.00
}

func (c *Comp) CurrentTime() sim.VTimeInSec {
	return c.Engine.CurrentTime()
}
