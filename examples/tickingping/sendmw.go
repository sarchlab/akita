package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// outPort is a helper that returns the "Out" port from the component.
func outPort(comp *modeling.Component[Spec, State, modeling.None]) messaging.Port {
	return comp.GetPortByName("Out")
}

// sendMW handles sending responses and ping requests.
type sendMW struct {
	comp *modeling.Component[Spec, State, modeling.None]
}

func (m *sendMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendRsp() || madeProgress
	madeProgress = m.sendPing() || madeProgress

	return madeProgress
}

func (m *sendMW) sendRsp() bool {
	state := &m.comp.State

	if len(state.CurrentTransactions) == 0 {
		return false
	}

	trans := state.CurrentTransactions[0]
	if trans.CycleLeft > 0 {
		return false
	}

	rsp := &pingRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   outPort(m.comp).AsRemote(),
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
		SeqID: trans.SeqID,
	}

	if !outPort(m.comp).CanSend() {
		return false
	}

	outPort(m.comp).Send(rsp)

	state.CurrentTransactions = state.CurrentTransactions[1:]

	return true
}

func (m *sendMW) sendPing() bool {
	state := &m.comp.State

	if state.NumPingNeedToSend == 0 {
		return false
	}

	pingMsg := &pingReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: outPort(m.comp).AsRemote(),
			Dst: state.PingDst,
		},
		SeqID: state.NextSeqID,
	}

	if !outPort(m.comp).CanSend() {
		return false
	}

	outPort(m.comp).Send(pingMsg)

	state.StartTimes = append(state.StartTimes, uint64(m.comp.CurrentTime()))
	state.NumPingNeedToSend--
	state.NextSeqID++

	return true
}
