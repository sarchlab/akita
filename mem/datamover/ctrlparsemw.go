package datamover

import (
	"log"

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

// Admit at most one head request per tick. A change of direction waits
// for all accepted moves to finish, including their destination write acks.
func (m *ctrlParseMW) parseFromCP() bool {
	state := &m.comp.State
	if len(state.Transactions) >= m.comp.Spec.MaxTransactions {
		return false
	}
	msg, ok := m.topPort().PeekIncoming()
	if !ok {
		return false
	}
	req, ok := msg.Payload.(datamoverprotocol.Req)
	if !ok {
		log.Panicf("can't process request of type %T", msg.Payload)
	}
	if len(state.Transactions) > 0 {
		first := &state.Transactions[0]
		if first.SrcSide != req.SrcSide || first.DstSide != req.DstSide {
			return false
		}
	}
	srcWidth := resolveByteGranularity(m.comp.Spec, req.SrcSide)
	addressMustBeAligned(req.SrcAddress, srcWidth)
	addressMustBeAligned(req.DstAddress, resolveByteGranularity(m.comp.Spec, req.DstSide))

	m.topPort().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".transaction"))
	m.topPort().RetrieveIncoming()
	state.Transactions = append(state.Transactions, dataMoverTransactionState{
		ReqID:         msg.ID,
		ReqSrc:        msg.Src,
		ReqDst:        msg.Dst,
		SrcAddress:    req.SrcAddress,
		DstAddress:    req.DstAddress,
		ByteSize:      req.ByteSize,
		SrcSide:       req.SrcSide,
		DstSide:       req.DstSide,
		NextReadAddr:  req.SrcAddress,
		NextWriteAddr: req.DstAddress,
		PendingRead:   make(map[uint64]pendingReadState),
		PendingWrite:  make(map[uint64]uint64),
		Buffer:        bufferState{Granularity: srcWidth},
		RecvTaskID:    m.comp.TaskTracker().StartReqIn(m.comp, msg),
	})
	return true
}

// Complete only the oldest transaction so responses retain admission order.
func (m *ctrlParseMW) finishTransaction() bool {
	state := &m.comp.State
	if len(state.Transactions) == 0 {
		return false
	}
	trans := &state.Transactions[0]
	if trans.NextWriteAddr-trans.DstAddress < trans.ByteSize ||
		len(trans.PendingRead) > 0 || len(trans.PendingWrite) > 0 {
		return false
	}
	rsp := messaging.Msg{ID: m.comp.NewID(), Src: trans.ReqDst,
		Dst: trans.ReqSrc, RspTo: trans.ReqID, Payload: datamoverprotocol.Rsp{}}
	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

	m.comp.TaskTracker().EndReqIn(trans.RecvTaskID)

	state.Transactions[0] = dataMoverTransactionState{}
	state.Transactions = state.Transactions[1:]
	return true
}
