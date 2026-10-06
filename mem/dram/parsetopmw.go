package dram

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type parseTopMW struct {
	comp *Comp
}

func (m *parseTopMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

// Handle runs the parseTop stage. Pause and Drain both stop accepting
// new traffic from the Top port; only Enabled DRAM accepts new
// transactions.
func (m *parseTopMW) Handle(_ timing.Event) bool {
	next := &m.comp.State
	if next.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}
	spec := m.comp.Spec

	return m.parseTop(&spec, next)
}

func (m *parseTopMW) parseTop(spec *Spec, next *state) bool {
	msgI, ok := m.topPort().PeekIncoming()
	if !ok {
		return false
	}

	ts := transactionState{ID: m.comp.NewID()}

	switch msgI.Payload.(type) {
	case memprotocol.ReadReq:
		msg := msgI

		ts.HasRead = true
		ts.ReadMsg = msg
	case memprotocol.WriteReq:
		msg := msgI

		ts.HasWrite = true
		ts.WriteMsg = msg
	default:
		panic(fmt.Sprintf("dram parseTop: unsupported message type %T", msgI.Payload))
	}

	// Split into sub-transactions
	transIdx := len(next.Transactions)
	splitTransaction(m.comp.NewID, spec, &ts)

	if !canPushSubTrans(next, len(ts.SubTransactions),
		spec.TransactionQueueSize) {
		return false
	}

	// A free sub-transaction-queue slot opened: the at-head wait the message
	// spent blocked on the SubTransQueue is over. This admission wait belongs to
	// the incoming-buffer task; req_in (opened at retrieve, below) covers only
	// the processing that follows.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtIncomingBuffer(msgI, m.comp),
		Kind:   tracing.MilestoneKindQueue,
		What:   m.comp.Name() + ".SubTransQueue",
	})

	ts.ArrivalTick = next.TickCount
	next.Transactions = append(next.Transactions, ts)
	pushSubTrans(next, transIdx)
	m.topPort().RetrieveIncoming()

	tracing.TraceReqReceive(m.comp, msgI)

	for _, st := range ts.SubTransactions {
		tracing.StartTask(m.comp, tracing.TaskStart{
			ID:       st.ID,
			ParentID: tracing.MsgIDAtReceiver(msgI, m.comp),
			Kind:     "sub-trans",
			What:     "sub-trans",
			Location: m.comp.Name() + ".SubTransQueue",
		})
	}

	return true
}
