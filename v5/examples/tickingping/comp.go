package tickingping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// PingReq is a ping request message.
type PingReq struct {
	sim.MsgMeta
	SeqID int
}

// PingRsp is a ping response message.
type PingRsp struct {
	sim.MsgMeta
	SeqID int
}

// Spec contains immutable configuration for the tickingping component.
type Spec struct{}

// pingTransactionState tracks an in-progress ping request with a countdown.
type pingTransactionState struct {
	SeqID     int            `json:"seq_id"`
	CycleLeft int            `json:"cycle_left"`
	ReqID     string         `json:"req_id"`
	ReqSrc    sim.RemotePort `json:"req_src"`
}

// State contains mutable runtime data for the tickingping component.
type State struct {
	StartTimes          []float64              `json:"start_times"`
	NextSeqID           int                    `json:"next_seq_id"`
	NumPingNeedToSend   int                    `json:"num_ping_need_to_send"`
	PingDst             sim.RemotePort         `json:"ping_dst"`
	CurrentTransactions []pingTransactionState `json:"current_transactions"`
}

// Comp is a ticking ping component that sends ping requests and responds to
// them.
type Comp struct {
	*modeling.Component[Spec, State]

	OutPort sim.Port
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
	msgI := m.OutPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	switch msg := msgI.(type) {
	case *PingReq:
		m.processingPingReq(msg)
	case *PingRsp:
		m.processingPingRsp(msg)
	default:
		panic("unknown message type")
	}

	return true
}

func (m *middleware) processingPingReq(msg *PingReq) {
	state := m.Component.GetState()

	trans := pingTransactionState{
		SeqID:     msg.SeqID,
		CycleLeft: 2,
		ReqID:     msg.ID,
		ReqSrc:    msg.Src,
	}
	state.CurrentTransactions = append(state.CurrentTransactions, trans)

	m.Component.SetState(state)
	m.OutPort.RetrieveIncoming()
}

func (m *middleware) processingPingRsp(msg *PingRsp) {
	state := m.Component.GetState()

	seqID := msg.SeqID
	startTime := state.StartTimes[seqID]
	currentTime := m.CurrentTime()
	duration := currentTime - sim.VTimeInSec(startTime)

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
	m.OutPort.RetrieveIncoming()
}

func (m *middleware) countDown() bool {
	state := m.Component.GetState()
	madeProgress := false

	for i := range state.CurrentTransactions {
		if state.CurrentTransactions[i].CycleLeft > 0 {
			state.CurrentTransactions[i].CycleLeft--
			madeProgress = true
		}
	}

	if madeProgress {
		m.Component.SetState(state)
	}

	return madeProgress
}

func (m *middleware) sendRsp() bool {
	state := m.Component.GetState()

	if len(state.CurrentTransactions) == 0 {
		return false
	}

	trans := state.CurrentTransactions[0]
	if trans.CycleLeft > 0 {
		return false
	}

	rsp := &PingRsp{
		MsgMeta: sim.MsgMeta{
			ID:    sim.GetIDGenerator().Generate(),
			Src:   m.OutPort.AsRemote(),
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
		SeqID: trans.SeqID,
	}

	err := m.OutPort.Send(rsp)
	if err != nil {
		return false
	}

	state.CurrentTransactions = state.CurrentTransactions[1:]
	m.Component.SetState(state)

	return true
}

func (m *middleware) sendPing() bool {
	state := m.Component.GetState()

	if state.NumPingNeedToSend == 0 {
		return false
	}

	pingMsg := &PingReq{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.OutPort.AsRemote(),
			Dst: state.PingDst,
		},
		SeqID: state.NextSeqID,
	}

	err := m.OutPort.Send(pingMsg)
	if err != nil {
		return false
	}

	state.StartTimes = append(state.StartTimes, float64(m.CurrentTime()))
	state.NumPingNeedToSend--
	state.NextSeqID++
	m.Component.SetState(state)

	return true
}
