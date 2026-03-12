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

// outPort is a helper that returns the "Out" port from the component.
func outPort(comp *modeling.Component[Spec, State]) sim.Port {
	return comp.GetPortByName("Out")
}

// sendMW handles sending responses and ping requests.
type sendMW struct {
	comp *modeling.Component[Spec, State]
}

func (m *sendMW) Name() string {
	return m.comp.Name()
}

func (m *sendMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *sendMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *sendMW) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *sendMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *sendMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendRsp() || madeProgress
	madeProgress = m.sendPing() || madeProgress

	return madeProgress
}

func (m *sendMW) sendRsp() bool {
	state := m.comp.GetState()

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
			Src:   outPort(m.comp).AsRemote(),
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
		SeqID: trans.SeqID,
	}

	err := outPort(m.comp).Send(rsp)
	if err != nil {
		return false
	}

	next := m.comp.GetNextState()
	next.CurrentTransactions = next.CurrentTransactions[1:]

	return true
}

func (m *sendMW) sendPing() bool {
	state := m.comp.GetState()

	if state.NumPingNeedToSend == 0 {
		return false
	}

	pingMsg := &PingReq{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: outPort(m.comp).AsRemote(),
			Dst: state.PingDst,
		},
		SeqID: state.NextSeqID,
	}

	err := outPort(m.comp).Send(pingMsg)
	if err != nil {
		return false
	}

	next := m.comp.GetNextState()
	next.StartTimes = append(next.StartTimes, float64(m.comp.CurrentTime()))
	next.NumPingNeedToSend--
	next.NextSeqID++

	return true
}

// receiveProcessMW handles receiving messages and counting down transactions.
type receiveProcessMW struct {
	comp *modeling.Component[Spec, State]
}

func (m *receiveProcessMW) Name() string {
	return m.comp.Name()
}

func (m *receiveProcessMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *receiveProcessMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *receiveProcessMW) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *receiveProcessMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *receiveProcessMW) Tick() bool {
	madeProgress := false

	madeProgress = m.countDown() || madeProgress
	madeProgress = m.processInput() || madeProgress

	return madeProgress
}

func (m *receiveProcessMW) processInput() bool {
	msgI := outPort(m.comp).PeekIncoming()
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

func (m *receiveProcessMW) processingPingReq(msg *PingReq) {
	next := m.comp.GetNextState()

	trans := pingTransactionState{
		SeqID:     msg.SeqID,
		CycleLeft: 2,
		ReqID:     msg.ID,
		ReqSrc:    msg.Src,
	}
	next.CurrentTransactions = append(next.CurrentTransactions, trans)

	outPort(m.comp).RetrieveIncoming()
}

func (m *receiveProcessMW) processingPingRsp(msg *PingRsp) {
	state := m.comp.GetState()

	seqID := msg.SeqID
	startTime := state.StartTimes[seqID]
	currentTime := m.comp.CurrentTime()
	duration := currentTime - sim.VTimeInSec(startTime)

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
	outPort(m.comp).RetrieveIncoming()
}

func (m *receiveProcessMW) countDown() bool {
	next := m.comp.GetNextState()
	madeProgress := false

	for i := range next.CurrentTransactions {
		if next.CurrentTransactions[i].CycleLeft > 0 {
			next.CurrentTransactions[i].CycleLeft--
			madeProgress = true
		}
	}

	return madeProgress
}
