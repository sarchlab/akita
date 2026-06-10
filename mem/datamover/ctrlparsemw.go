package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// ctrlParseMW handles control port parsing and transaction completion.
	"github.com/sarchlab/akita/v5/messaging"
)

type ctrlParseMW struct {
	comp *modeling.Component[Spec, State, modeling.None]
}

// topPort is the workload-request port the data mover listens on for
// datamoverprotocol.DataMoveRequest messages. (It was historically named "Control" but
// that name is now reserved for the uniform control protocol.)
func (m *ctrlParseMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

// Tick runs finishTransaction and parseFromCP. Paused data movers
// freeze entirely; draining ones finish the current transaction but
// don't accept new ones.
func (m *ctrlParseMW) Tick() bool {
	if m.comp.State.ControlState == control.StatePaused {
		return false
	}

	madeProgress := false

	madeProgress = m.finishTransaction() || madeProgress
	if m.comp.State.ControlState == control.StateEnabled {
		madeProgress = m.parseFromCP() || madeProgress
	}

	return madeProgress
}

// parseFromCP retrieves Msg from ctrlPort.
func (m *ctrlParseMW) parseFromCP() bool {
	reqI := m.topPort().RetrieveIncoming()
	if reqI == nil {
		return false
	}

	req, ok := reqI.(datamoverprotocol.DataMoveRequest)
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

	// All reads/writes have been issued, but the move is only truly complete
	// once every one is acknowledged. Completing earlier would report the copy
	// done (and let a Drain quiesce) while destination writes are still in
	// flight and their pending metadata is about to be discarded.
	if len(trans.PendingRead) > 0 || len(trans.PendingWrite) > 0 {
		return false
	}

	rsp := datamoverprotocol.DataMoveResponse{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   trans.ReqDst,
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
	}

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

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
