package dram

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type parseTopMW struct {
	comp    *modeling.Component[Spec, State]
	topPort sim.Port
}

// Tick runs the parseTop stage.
func (m *parseTopMW) Tick() bool {
	next := m.comp.GetNextState()
	spec := m.comp.GetSpec()

	return m.parseTop(&spec, next)
}

func (m *parseTopMW) parseTop(spec *Spec, next *State) bool {
	msgI := m.topPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	ts := transactionState{}

	switch msg := msgI.(type) {
	case *mem.ReadReq:
		ts.HasRead = true
		ts.ReadMsg = *msg
	case *mem.WriteReq:
		ts.HasWrite = true
		ts.WriteMsg = *msg
	}

	// Assign internal address
	globalAddr := transactionGlobalAddress(&ts)
	ts.InternalAddress = convertExternalToInternal(spec, globalAddr)

	// Split into sub-transactions
	transIdx := len(next.Transactions)
	splitTransaction(spec, &ts, transIdx)

	if !canPushSubTrans(next, len(ts.SubTransactions),
		spec.TransactionQueueSize) {
		return false
	}

	ts.ArrivalTick = next.TickCount
	next.Transactions = append(next.Transactions, ts)
	pushSubTrans(next, transIdx)
	m.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msgI, m.comp)

	for _, st := range ts.SubTransactions {
		tracing.StartTaskWithSpecificLocation(
			st.ID,
			tracing.MsgIDAtReceiver(msgI, m.comp),
			m.comp,
			"sub-trans",
			"sub-trans",
			m.comp.Name()+".SubTransQueue",
			nil,
		)
	}

	return true
}
