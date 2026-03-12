package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// ctrlParseMW handles control port parsing and transaction completion.
type ctrlParseMW struct {
	comp *modeling.Component[Spec, State]
}

// NamedHookable delegation methods.

func (m *ctrlParseMW) Name() string {
	return m.comp.Name()
}

func (m *ctrlParseMW) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *ctrlParseMW) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *ctrlParseMW) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *ctrlParseMW) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *ctrlParseMW) ctrlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

// Tick runs finishTransaction and parseFromCP.
func (m *ctrlParseMW) Tick() bool {
	madeProgress := false

	madeProgress = m.finishTransaction() || madeProgress
	madeProgress = m.parseFromCP() || madeProgress

	return madeProgress
}

// parseFromCP retrieves Msg from ctrlPort.
func (m *ctrlParseMW) parseFromCP() bool {
	reqI := m.ctrlPort().RetrieveIncoming()
	if reqI == nil {
		return false
	}

	req, ok := reqI.(*DataMoveRequest)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(reqI))
	}

	cur := m.comp.GetState()
	if cur.CurrentTransaction.Active {
		return false
	}

	spec := m.comp.GetSpec()

	srcByteGranularity := resolveByteGranularity(spec, req.SrcSide)
	addressMustBeAligned(req.SrcAddress, srcByteGranularity)

	dstByteGranularity := resolveByteGranularity(spec, req.DstSide)
	addressMustBeAligned(req.DstAddress, dstByteGranularity)

	next := m.comp.GetNextState()
	next.SrcSide = string(req.SrcSide)
	next.DstSide = string(req.DstSide)
	next.SrcByteGranularity = srcByteGranularity
	next.DstByteGranularity = dstByteGranularity

	next.CurrentTransaction = dataMoverTransactionState{
		Active:        true,
		ReqID:         req.ID,
		ReqSrc:        req.Src,
		ReqDst:        req.Dst,
		SrcAddress:    req.SrcAddress,
		DstAddress:    req.DstAddress,
		ByteSize:      req.ByteSize,
		SrcSide:       string(req.SrcSide),
		DstSide:       string(req.DstSide),
		NextReadAddr:  req.SrcAddress,
		NextWriteAddr: req.DstAddress,
		PendingRead:   make(map[string]pendingReadState),
		PendingWrite:  make(map[string]pendingWriteState),
	}

	next.Buffer = bufferState{
		Granularity: srcByteGranularity,
	}

	tracing.TraceReqReceive(req, m)

	return true
}

// finishTransaction finishes the current transaction.
func (m *ctrlParseMW) finishTransaction() bool {
	cur := m.comp.GetState()
	if !cur.CurrentTransaction.Active {
		return false
	}

	curTrans := &cur.CurrentTransaction

	if curTrans.NextWriteAddr < curTrans.DstAddress+curTrans.ByteSize {
		return false
	}

	rsp := &DataMoveResponse{
		MsgMeta: sim.MsgMeta{
			ID:    sim.GetIDGenerator().Generate(),
			Src:   curTrans.ReqDst,
			Dst:   curTrans.ReqSrc,
			RspTo: curTrans.ReqID,
		},
	}

	err := m.ctrlPort().Send(rsp)
	if err != nil {
		return false
	}

	// Reset transaction
	next := m.comp.GetNextState()
	next.CurrentTransaction = dataMoverTransactionState{
		PendingRead:  make(map[string]pendingReadState),
		PendingWrite: make(map[string]pendingWriteState),
	}
	next.Buffer = bufferState{
		Offset:      alignAddress(curTrans.SrcAddress, cur.SrcByteGranularity),
		Granularity: cur.SrcByteGranularity,
	}

	tracing.TraceReqComplete(rsp, m)

	return true
}

