package tickingping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim"
)

// PingReqPayload is the payload for a ping request message.
type PingReqPayload struct {
	SeqID int
}

// PingRspPayload is the payload for a ping response message.
type PingRspPayload struct {
	SeqID int
}

type pingTransaction struct {
	req       *sim.Msg
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

	switch msg.Payload.(type) {
	case *PingReqPayload:
		m.processingPingReq(msg)
	case *PingRspPayload:
		m.processingPingRsp(msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (m *middleware) processingPingReq(
	msg *sim.Msg,
) {
	trans := &pingTransaction{
		req:       msg,
		cycleLeft: 2,
	}
	m.currentTransactions = append(m.currentTransactions, trans)
	m.OutPort.RetrieveIncoming()
}

func (m *middleware) processingPingRsp(
	msg *sim.Msg,
) {
	payload := sim.MsgPayload[PingRspPayload](msg)
	seqID := payload.SeqID
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

	reqPayload := sim.MsgPayload[PingReqPayload](trans.req)
	rsp := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.OutPort.AsRemote(),
			Dst: trans.req.Src,
		},
		RspTo: trans.req.ID,
		Payload: &PingRspPayload{
			SeqID: reqPayload.SeqID,
		},
	}

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

	pingMsg := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.OutPort.AsRemote(),
			Dst: m.pingDst,
		},
		Payload: &PingReqPayload{
			SeqID: m.nextSeqID,
		},
	}

	err := m.OutPort.Send(pingMsg)
	if err != nil {
		return false
	}

	m.startTime = append(m.startTime, m.CurrentTime())
	m.numPingNeedToSend--
	m.nextSeqID++

	return true
}
