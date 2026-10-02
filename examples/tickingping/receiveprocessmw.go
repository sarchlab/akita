package tickingping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/timing"
)

// receiveProcessMW handles receiving messages and counting down transactions.
type receiveProcessMW struct {
	comp *Comp
}

// Handle counts down every ping being answered and takes at most one incoming
// message per tick.
func (m *receiveProcessMW) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.countDown() || madeProgress
	madeProgress = m.processInput() || madeProgress

	return madeProgress
}

func (m *receiveProcessMW) processInput() bool {
	msgI, ok := m.comp.Ports.Out.RetrieveIncoming()
	if !ok {
		return false
	}

	switch msg := msgI.(type) {
	case pingReq:
		m.processPingReq(msg)
	case pingRsp:
		m.processPingRsp(msg)
	default:
		panic("unknown message type")
	}

	return true
}

// processPingReq starts answering a ping; the response goes out two cycles
// later.
func (m *receiveProcessMW) processPingReq(msg pingReq) {
	state := &m.comp.State

	state.CurrentTransactions = append(state.CurrentTransactions,
		pingTransactionState{
			SeqID:     msg.SeqID,
			CycleLeft: 2,
			ReqID:     msg.ID,
			ReqSrc:    msg.Src,
		})
}

// processPingRsp prints the round-trip time of the ping it answers.
func (m *receiveProcessMW) processPingRsp(msg pingRsp) {
	startTime := m.comp.State.StartTimes[msg.SeqID]
	duration := uint64(m.comp.CurrentTime()) - startTime

	fmt.Printf("Ping %d, %d ps\n", msg.SeqID, duration)
}

func (m *receiveProcessMW) countDown() bool {
	state := &m.comp.State
	madeProgress := false

	for i := range state.CurrentTransactions {
		if state.CurrentTransactions[i].CycleLeft > 0 {
			state.CurrentTransactions[i].CycleLeft--
			madeProgress = true
		}
	}

	return madeProgress
}
