package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// ctrlParseMW handles control port parsing and transaction completion.
type ctrlParseMW struct {
	comp *Comp
}

// topPort is the workload-request port the data mover listens on for
// datamoverprotocol.Req messages. (It was historically named "Control" but
// that name is now reserved for the uniform control protocol.)
func (m *ctrlParseMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

// Handle runs finishTransaction and parseFromCP. Paused data movers
// freeze entirely; draining ones finish the current transaction but
// don't accept new ones.
func (m *ctrlParseMW) Handle(_ timing.Event) bool {
	if m.comp.State.ControlState == controlStatePaused {
		return false
	}

	madeProgress := false

	madeProgress = m.finishTransaction() || madeProgress
	if m.comp.State.ControlState == controlStateEnabled {
		madeProgress = m.parseFromCP() || madeProgress
	}

	return madeProgress
}

// parseFromCP admits the head request from topPort. The data mover runs a
// single transaction at a time, so a request that arrives while one is already
// running must wait at the head of the buffer rather than be retrieved and
// dropped. The head is therefore peeked first; the message is only retrieved
// (and admitted) once the single transaction slot is free.
func (m *ctrlParseMW) parseFromCP() bool {
	state := &m.comp.State
	if state.CurrentTransaction.Active {
		return false
	}

	reqI, ok := m.topPort().PeekIncoming()
	if !ok {
		return false
	}
	_, ok = reqI.Payload.(datamoverprotocol.Req)
	req := reqI
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(reqI.Payload))
	}

	spec := m.comp.Spec

	srcByteGranularity := resolveByteGranularity(spec, req.Payload.(datamoverprotocol.Req).SrcSide)
	addressMustBeAligned(req.Payload.(datamoverprotocol.Req).SrcAddress, srcByteGranularity)

	dstByteGranularity := resolveByteGranularity(spec, req.Payload.(datamoverprotocol.Req).DstSide)
	addressMustBeAligned(req.Payload.(datamoverprotocol.Req).DstAddress, dstByteGranularity)

	// The slot is free and this request is being admitted. Attribute the
	// at-head admission wait to the buffer task (which the port closes at
	// retrieve), using the task ID stored in the port's head entry.
	m.topPort().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".transaction"))

	m.topPort().RetrieveIncoming()

	state.SrcSide = req.Payload.(datamoverprotocol.Req).SrcSide
	state.DstSide = req.Payload.(datamoverprotocol.Req).DstSide
	state.SrcByteGranularity = srcByteGranularity
	state.DstByteGranularity = dstByteGranularity

	state.CurrentTransaction = dataMoverTransactionState{
		Active:        true,
		ReqID:         req.ID,
		ReqSrc:        req.Src,
		ReqDst:        req.Dst,
		SrcAddress:    req.Payload.(datamoverprotocol.Req).SrcAddress,
		DstAddress:    req.Payload.(datamoverprotocol.Req).DstAddress,
		ByteSize:      req.Payload.(datamoverprotocol.Req).ByteSize,
		SrcSide:       req.Payload.(datamoverprotocol.Req).SrcSide,
		DstSide:       req.Payload.(datamoverprotocol.Req).DstSide,
		NextReadAddr:  req.Payload.(datamoverprotocol.Req).SrcAddress,
		NextWriteAddr: req.Payload.(datamoverprotocol.Req).DstAddress,
		PendingRead:   make(map[uint64]pendingReadState),
		PendingWrite:  make(map[uint64]pendingWriteState),
	}

	state.Buffer = bufferState{
		Granularity: srcByteGranularity,
	}

	state.CurrentTransaction.RecvTaskID = m.comp.TaskTracker().StartReqIn(m.comp, req)

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

	rsp := messaging.Msg{ID: m.comp.NewID(),
		Src:   trans.ReqDst,
		Dst:   trans.ReqSrc,
		RspTo: trans.ReqID, Payload: datamoverprotocol.Rsp{}}

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

	m.comp.TaskTracker().EndReqIn(trans.RecvTaskID)

	// Reset transaction
	state.CurrentTransaction = dataMoverTransactionState{
		PendingRead:  make(map[uint64]pendingReadState),
		PendingWrite: make(map[uint64]pendingWriteState),
	}
	state.Buffer = bufferState{
		Offset:      alignAddress(trans.SrcAddress, state.SrcByteGranularity),
		Granularity: state.SrcByteGranularity,
	}

	return true
}
