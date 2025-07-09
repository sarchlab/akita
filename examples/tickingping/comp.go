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
	sim.MiddlewareHolder

	OutPort sim.Port

	currentTransactions []*pingTransaction
	startTime           []sim.VTimeInSec
	numPingNeedToSend   int
	nextSeqID           int
	pingDst             sim.RemotePort
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
	case *PingReq:
		m.processingPingReq(msg)
	case *PingRsp:
		m.processingPingRsp(msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (m *middleware) processingPingReq(
	ping *PingReq,
) {
	trans := &pingTransaction{
		req:       ping,
		cycleLeft: 2,
	}
	m.currentTransactions = append(m.currentTransactions, trans)
	m.OutPort.RetrieveIncoming()
}

func (m *middleware) processingPingRsp(
	msg *PingRsp,
) {
	seqID := msg.SeqID
	startTime := m.startTime[seqID]
	currentTime := m.CurrentTime()
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

	rsp := &PingRsp{
		SeqID: trans.req.SeqID,
	}
	rsp.Src = m.OutPort.AsRemote()
	rsp.Dst = trans.req.Src

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

	PingReq := &PingReq{
		SeqID: m.nextSeqID,
	}
	PingReq.Src = m.OutPort.AsRemote()
	PingReq.Dst = m.pingDst

	err := m.OutPort.Send(PingReq)
	if err != nil {
		return false
	}

	m.startTime = append(m.startTime, m.CurrentTime())
	m.numPingNeedToSend--
	m.nextSeqID++

	return true
}
