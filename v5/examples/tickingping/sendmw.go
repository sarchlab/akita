package tickingping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// outPort is a helper that returns the "Out" port from the component.
func outPort(comp *modeling.Component[Spec, State]) sim.Port {
	return comp.GetPortByName("Out")
}

// sendMW handles sending responses and ping requests.
type sendMW struct {
	comp *modeling.Component[Spec, State]
}

func (m *sendMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendRsp() || madeProgress
	madeProgress = m.sendPing() || madeProgress

	return madeProgress
}

func (m *sendMW) sendRsp() bool {
	state := m.comp.GetNextState()

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

	state.CurrentTransactions = state.CurrentTransactions[1:]

	return true
}

func (m *sendMW) sendPing() bool {
	state := m.comp.GetNextState()

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

	state.StartTimes = append(state.StartTimes, uint64(m.comp.CurrentTime()))
	state.NumPingNeedToSend--
	state.NextSeqID++

	return true
}
