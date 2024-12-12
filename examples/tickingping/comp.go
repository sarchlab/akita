package tickingping

import (
	"fmt"

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
	cloneMsg.MsgMeta.ID = id.Generate()

	return &cloneMsg
}

func (p PingReq) GenerateRsp() PingRsp {
	rsp := PingRsp{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: p.MsgMeta.Dst,
			Dst: p.MsgMeta.Src,
		},
		RspTo: p.MsgMeta.ID,
		SeqID: p.SeqID,
	}

	return rsp
}

type PingRsp struct {
	modeling.MsgMeta

	RspTo string
	SeqID int
}

func (p PingRsp) Meta() modeling.MsgMeta {
	return p.MsgMeta
}

func (p PingRsp) Clone() modeling.Msg {
	cloneMsg := p
	cloneMsg.MsgMeta.ID = id.Generate()

	return cloneMsg
}

func (p PingRsp) GetRspTo() string {
	return p.RspTo
}

type pingTransaction struct {
	req       PingReq
	cycleLeft int
}

type Comp struct {
	*modeling.TickingComponent
	modeling.MiddlewareHolder

	OutPort modeling.Port

	currentTransactions []*pingTransaction
	startTime           []timing.VTimeInSec
	numPingNeedToSend   int
	nextSeqID           int
	pingDst             modeling.RemotePort
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendRsp() || madeProgress
	madeProgress = m.sendPing() || madeProgress
	madeProgress = m.countDown() || madeProgress
	madeProgress = m.processInput() || madeProgress

	return madeProgress
}

func (m *middleware) processInput() bool {
	msg := m.OutPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case PingReq:
		m.processingPingReq(msg)
	case PingRsp:
		m.processingPingRsp(msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (m *middleware) processingPingReq(
	ping PingReq,
) {
	trans := &pingTransaction{
		req:       ping,
		cycleLeft: 2,
	}
	m.currentTransactions = append(m.currentTransactions, trans)
	m.OutPort.RetrieveIncoming()
}

func (m *middleware) processingPingRsp(
	msg PingRsp,
) {
	seqID := msg.SeqID
	startTime := m.startTime[seqID]
	currentTime := m.Now()
	duration := currentTime - startTime

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
	m.OutPort.RetrieveIncoming()
}

func (m *middleware) countDown() bool {
	madeProgress := false

	for _, trans := range m.currentTransactions {
		if trans.cycleLeft > 0 {
			trans.cycleLeft--
			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) sendRsp() bool {
	if len(m.currentTransactions) == 0 {
		return false
	}

	trans := m.currentTransactions[0]
	if trans.cycleLeft > 0 {
		return false
	}

	rsp := trans.req.GenerateRsp()

	err := m.OutPort.Send(rsp)
	if err != nil {
		return false
	}

	m.currentTransactions = m.currentTransactions[1:]

	return true
}

func (m *middleware) sendPing() bool {
	if m.numPingNeedToSend == 0 {
		return false
	}

	pingReq := PingReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.OutPort.AsRemote(),
			Dst: m.pingDst,
		},
		SeqID: m.nextSeqID,
	}

	err := m.OutPort.Send(pingReq)
	if err != nil {
		return false
	}

	m.startTime = append(m.startTime, m.Now())
	m.numPingNeedToSend--
	m.nextSeqID++

	return true
}
