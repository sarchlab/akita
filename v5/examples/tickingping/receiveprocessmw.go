package tickingping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// receiveProcessMW handles receiving messages and counting down transactions.
type receiveProcessMW struct {
	comp *modeling.Component[Spec, State]
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
	state := m.comp.GetNextState()

	trans := pingTransactionState{
		SeqID:     msg.SeqID,
		CycleLeft: 2,
		ReqID:     msg.ID,
		ReqSrc:    msg.Src,
	}
	state.CurrentTransactions = append(state.CurrentTransactions, trans)

	outPort(m.comp).RetrieveIncoming()
}

func (m *receiveProcessMW) processingPingRsp(msg *PingRsp) {
	state := m.comp.GetNextState()

	seqID := msg.SeqID
	startTime := state.StartTimes[seqID]
	currentTime := m.comp.CurrentTime()
	duration := currentTime - sim.VTimeInSec(startTime)

	fmt.Printf("Ping %d, %.2f\n", seqID, duration)
	outPort(m.comp).RetrieveIncoming()
}

func (m *receiveProcessMW) countDown() bool {
	state := m.comp.GetNextState()
	madeProgress := false

	for i := range state.CurrentTransactions {
		if state.CurrentTransactions[i].CycleLeft > 0 {
			state.CurrentTransactions[i].CycleLeft--
			madeProgress = true
		}
	}

	return madeProgress
}
