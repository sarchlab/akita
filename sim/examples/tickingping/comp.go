package tickingping

import (
	"fmt"

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

func (p *PingRsp) GenerateRsp() sim.Rsp {
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

type pingTransaction struct {
	req       *PingReq
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
	case *PingReq:
		c.processingPingMsg(msg)
	case *PingRsp:
		c.processingPingRsp(msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (c *Comp) processingPingMsg(
	ping *PingReq,
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

	pingMsg := &PingReq{
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
