package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// sendMW handles sending responses and ping requests.
type sendMW struct {
	comp *Comp
}

// Handle sends at most one response and one ping per tick.
func (m *sendMW) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.sendRsp() || madeProgress
	madeProgress = m.sendPing() || madeProgress

	return madeProgress
}

func (m *sendMW) sendRsp() bool {
	state := &m.comp.State
	out := m.comp.Ports.Out

	if len(state.CurrentTransactions) == 0 {
		return false
	}

	trans := state.CurrentTransactions[0]
	if trans.CycleLeft > 0 {
		return false
	}

	if !out.CanSend() {
		return false
	}

	out.Send(pingRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    m.comp.Simulation().NewID(),
			Src:   out.AsRemote(),
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
		SeqID: trans.SeqID,
	})

	state.CurrentTransactions = state.CurrentTransactions[1:]

	return true
}

func (m *sendMW) sendPing() bool {
	state := &m.comp.State
	spec := m.comp.Spec()
	out := m.comp.Ports.Out

	if state.NextSeqID >= spec.NumPings {
		return false
	}

	if !out.CanSend() {
		return false
	}

	out.Send(pingReq{
		MsgMeta: messaging.MsgMeta{
			ID:  m.comp.Simulation().NewID(),
			Src: out.AsRemote(),
			Dst: spec.PingDst,
		},
		SeqID: state.NextSeqID,
	})

	state.StartTimes = append(state.StartTimes, uint64(m.comp.CurrentTime()))
	state.NextSeqID++

	return true
}
