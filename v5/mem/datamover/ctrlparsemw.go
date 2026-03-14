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

	state := m.comp.GetNextState()
	if state.CurrentTransaction.Active {
		return false
	}

	spec := m.comp.GetSpec()

	srcByteGranularity := resolveByteGranularity(spec, req.SrcSide)
	addressMustBeAligned(req.SrcAddress, srcByteGranularity)

	dstByteGranularity := resolveByteGranularity(spec, req.DstSide)
	addressMustBeAligned(req.DstAddress, dstByteGranularity)

	state.SrcSide = string(req.SrcSide)
	state.DstSide = string(req.DstSide)
	state.SrcByteGranularity = srcByteGranularity
	state.DstByteGranularity = dstByteGranularity

	state.CurrentTransaction = dataMoverTransactionState{
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

	state.Buffer = bufferState{
		Granularity: srcByteGranularity,
	}

	tracing.TraceReqReceive(req, m.comp)

	return true
}

// finishTransaction finishes the current transaction.
func (m *ctrlParseMW) finishTransaction() bool {
	state := m.comp.GetNextState()
	if !state.CurrentTransaction.Active {
		return false
	}

	trans := &state.CurrentTransaction

	if trans.NextWriteAddr < trans.DstAddress+trans.ByteSize {
		return false
	}

	rsp := &DataMoveResponse{
		MsgMeta: sim.MsgMeta{
			ID:    sim.GetIDGenerator().Generate(),
			Src:   trans.ReqDst,
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
	}

	err := m.ctrlPort().Send(rsp)
	if err != nil {
		return false
	}

	// Reset transaction
	state.CurrentTransaction = dataMoverTransactionState{
		PendingRead:  make(map[string]pendingReadState),
		PendingWrite: make(map[string]pendingWriteState),
	}
	state.Buffer = bufferState{
		Offset:      alignAddress(trans.SrcAddress, state.SrcByteGranularity),
		Granularity: state.SrcByteGranularity,
	}

	tracing.TraceReqComplete(rsp, m.comp)

	return true
}

