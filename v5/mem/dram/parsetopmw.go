package dram

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type parseTopMW struct {
	comp    *modeling.Component[Spec, State]
	topPort sim.Port
}

// Name delegates to the underlying component.
func (m *parseTopMW) Name() string {
	return m.comp.Name()
}

// AcceptHook delegates to the underlying component.
func (m *parseTopMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

// Hooks delegates to the underlying component.
func (m *parseTopMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

// NumHooks delegates to the underlying component.
func (m *parseTopMW) NumHooks() int {
	return m.comp.NumHooks()
}

// InvokeHook delegates to the underlying component.
func (m *parseTopMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

// Tick runs the parseTop stage.
func (m *parseTopMW) Tick() bool {
	curVal := m.comp.GetState()
	cur := &curVal
	next := m.comp.GetNextState()
	spec := m.comp.GetSpec()

	return m.parseTop(&spec, cur, next)
}

func (m *parseTopMW) parseTop(spec *Spec, cur *State, next *State) bool {
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

	if !canPushSubTrans(cur, len(ts.SubTransactions),
		spec.TransactionQueueSize) {
		return false
	}

	next.Transactions = append(next.Transactions, ts)
	pushSubTrans(next, transIdx)
	m.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msgI, m)

	for _, st := range ts.SubTransactions {
		tracing.StartTaskWithSpecificLocation(
			st.ID,
			tracing.MsgIDAtReceiver(msgI, m),
			m,
			"sub-trans",
			"sub-trans",
			m.comp.Name()+".SubTransQueue",
			nil,
		)
	}

	return true
}
