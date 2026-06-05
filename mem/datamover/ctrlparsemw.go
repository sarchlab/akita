package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// ctrlParseMW handles control port parsing and transaction completion.
	"github.com/sarchlab/akita/v5/messaging"
)

type ctrlParseMW struct {
	comp *modeling.Component[Spec, State, modeling.None]
}

func (m *ctrlParseMW) ctrlPort() messaging.Port {
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

	req, ok := reqI.(DataMoveRequest)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(reqI))
	}

	state := &m.comp.State
	if state.CurrentTransaction.Active {
		return false
	}

	spec := m.comp.Spec()

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
		PendingRead:   make(map[uint64]pendingReadState),
		PendingWrite:  make(map[uint64]pendingWriteState),
	}

	state.Buffer = bufferState{
		Granularity: srcByteGranularity,
	}

	tracing.TraceReqReceive(m.comp, req)

	return true
}

// finishTransaction finishes the current transaction.
func (m *ctrlParseMW) finishTransaction() bool {
	state := &m.comp.State
	if !state.CurrentTransaction.Active {
		return false
	}

	trans := &state.CurrentTransaction

	if trans.NextWriteAddr < trans.DstAddress+trans.ByteSize {
		return false
	}

	rsp := DataMoveResponse{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   trans.ReqDst,
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(rsp)

	// Reset transaction
	state.CurrentTransaction = dataMoverTransactionState{
		PendingRead:  make(map[uint64]pendingReadState),
		PendingWrite: make(map[uint64]pendingWriteState),
	}
	state.Buffer = bufferState{
		Offset:      alignAddress(trans.SrcAddress, state.SrcByteGranularity),
		Granularity: state.SrcByteGranularity,
	}

	tracing.TraceReqComplete(m.comp, rsp)

	return true
}
