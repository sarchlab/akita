package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// respondPipelineMW handles translation responses and bottom-port responses.
type respondPipelineMW struct {
	comp *Comp
}

func (m *respondPipelineMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *respondPipelineMW) bottomPort() messaging.Port {
	return m.comp.Ports.Bottom
}

func (m *respondPipelineMW) translationPort() messaging.Port {
	return m.comp.Ports.Translation
}

// Handle runs the respond pipeline: respond + parseTranslation. It is
// gated by ControlState — paused agents do not advance in-flight
// transactions; draining and enabled agents do, so a Drain can
// converge.
func (m *respondPipelineMW) Handle(_ timing.Event) bool {
	if m.comp.State.ControlState == controlStatePaused {
		return false
	}

	madeProgress := false

	spec := m.comp.Spec

	for range spec.NumReqPerCycle {
		madeProgress = m.respond() || madeProgress
	}

	madeProgress = m.releaseHeld() || madeProgress

	for range spec.NumReqPerCycle {
		madeProgress = m.parseTranslation() || madeProgress
	}

	return madeProgress
}

func (m *respondPipelineMW) parseTranslation() bool {
	rspI, ok := m.translationPort().PeekIncoming()
	if !ok {
		return false
	}

	rsp := rspI
	nextState := &m.comp.State
	transIdx := findTransactionByReqID(nextState.Transactions, rsp.RspTo)

	if transIdx < 0 {
		m.translationPort().RetrieveIncoming()
		return true
	}

	nextTrans := &nextState.Transactions[transIdx]
	reqState := nextTrans.IncomingReqs[0]
	translated := rsp.Payload.(vmprotocol.TranslationRsp).Page

	page, proceed := translated, true
	if hook := m.comp.Resources.MigrationHook; hook != nil {
		page, proceed = hook.Resolve(m.comp, reqState.PID, reqState.Address,
			translated)
	}

	var translatedReq messaging.Msg
	if proceed {
		translatedReq = m.translatedReq(reqState, page)

		if !m.bottomPort().CanSend() {
			return false
		}
	}

	nextTrans.TranslationDone = true
	nextTrans.Page = translated

	// Trace before removeTransaction, which shifts the transactions, so the
	// translation identity is read from the correct transaction.
	m.traceTranslationDone(nextTrans, reqState)

	if proceed {
		m.sendTranslated(reqState, translatedReq)
	} else {
		nextState.HeldReqs = append(nextState.HeldReqs,
			heldReqState{Req: reqState, Page: translated})
		nextState.NumHeldReqs++
	}

	nextTrans.IncomingReqs = nextTrans.IncomingReqs[1:]

	if len(nextTrans.IncomingReqs) == 0 {
		removeTransaction(nextState, transIdx)
	}

	m.translationPort().RetrieveIncoming()

	return true
}

// releaseHeld retries, in arrival order, the accesses held by the migration
// hook. It stops at the first access that must still wait or cannot be sent,
// so held accesses leave in order.
func (m *respondPipelineMW) releaseHeld() bool {
	nextState := &m.comp.State
	hook := m.comp.Resources.MigrationHook
	madeProgress := false

	for len(nextState.HeldReqs) > 0 {
		held := nextState.HeldReqs[0]

		page, proceed := hook.Resolve(m.comp, held.Req.PID, held.Req.Address,
			held.Page)
		if !proceed {
			return madeProgress
		}

		translatedReq := m.translatedReq(held.Req, page)
		if !m.bottomPort().CanSend() {
			return madeProgress
		}

		m.comp.TaskTracker().AddMilestone(tasks.Milestone{
			TaskID: held.Req.RecvTaskID,
			Kind:   tasks.MilestoneKindDependency,
			What:   "migration",
		})

		m.sendTranslated(held.Req, translatedReq)
		nextState.HeldReqs = nextState.HeldReqs[1:]
		madeProgress = true
	}

	return madeProgress
}

// translatedReq creates the request that accesses the physical address of
// page.
func (m *respondPipelineMW) translatedReq(
	reqState incomingReqState,
	page vm.Page,
) messaging.Msg {
	return createTranslatedReq(m.comp.NewID, reqState, page,
		m.comp.Spec.Log2PageSize, m.bottomPort().AsRemote(),
		m.comp.Spec.Bottom)
}

// sendTranslated sends a translated request to the memory system. The caller
// has checked that the Bottom port can send.
func (m *respondPipelineMW) sendTranslated(
	reqState incomingReqState,
	translatedReq messaging.Msg,
) {
	// The translated request leaves through the Bottom port; this
	// network-busy wait belongs to the top req_in, not to the downstream
	// req_out (which this component sends, so its receiver-side ID would
	// be a phantom task).
	m.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: reqState.RecvTaskID,
		Kind:   tasks.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	sendTaskID := m.comp.TaskTracker().StartReqOut(m.comp, translatedReq,
		reqState.RecvTaskID)
	m.bottomPort().Send(translatedReq)

	nextState := &m.comp.State
	reqToBot := buildReqToBottom(reqState, translatedReq)
	reqToBot.SendTaskID = sendTaskID
	nextState.InflightReqToBottom = append(
		nextState.InflightReqToBottom, reqToBot)

	if hook := m.comp.Resources.MigrationHook; hook != nil {
		hook.Issued(reqState.PID, reqState.Address, reqToBot.ReqToBottomAddr)
	}
}

// traceTranslationDone records, on the top req_in task, the milestone for a
// completed translation and finalizes the translation req_out. trans must
// still be live in State.Transactions.
func (m *respondPipelineMW) traceTranslationDone(
	trans *transactionState,
	reqState incomingReqState,
) {
	m.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: reqState.RecvTaskID,
		Kind:   tasks.MilestoneKindTranslation,
		What:   "translation",
	})

	m.comp.TaskTracker().EndReqOut(trans.TranslationTaskID)
}

//nolint:funlen,gocyclo
func (m *respondPipelineMW) respond() bool {
	rspI, ok := m.bottomPort().PeekIncoming()
	if !ok {
		return false
	}

	nextState := &m.comp.State

	var (
		reqFromTopState reqToBottomState
		rspToTop        messaging.Msg
	)

	reqInBottom := false

	switch content := rspI.Payload.(type) {
	case memprotocol.DataReadyRsp:
		rsp := rspI

		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = messaging.Msg{ID: m.comp.NewID(),
				Src:          m.topPort().AsRemote(),
				Dst:          reqFromTopState.ReqFromTopSrc,
				RspTo:        reqFromTopState.ReqFromTopID,
				TrafficBytes: len(content.Data) + 4,
				Payload: memprotocol.DataReadyRsp{
					Data: content.Data,
				}}
			m.comp.TaskTracker().AddMilestone(tasks.Milestone{
				TaskID: reqFromTopState.RecvTaskID,
				Kind:   tasks.MilestoneKindData,
				What:   "data",
			})
		}
	case memprotocol.WriteDoneRsp:
		rsp := rspI

		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = messaging.Msg{ID: m.comp.NewID(),
				Src:          m.topPort().AsRemote(),
				Dst:          reqFromTopState.ReqFromTopSrc,
				RspTo:        reqFromTopState.ReqFromTopID,
				TrafficBytes: 4,
				Payload:      memprotocol.WriteDoneRsp{}}
			m.comp.TaskTracker().AddMilestone(tasks.Milestone{
				TaskID: reqFromTopState.RecvTaskID,
				Kind:   tasks.MilestoneKindSubTask,
				What:   "subtask",
			})
		}
	default:
		log.Panicf("cannot handle respond of type %s", fmt.Sprintf("%T", rspI.Payload))
	}

	if reqInBottom {
		if !m.topPort().CanSend() {
			return false
		}

		m.topPort().Send(rspToTop)

		m.comp.TaskTracker().AddMilestone(tasks.Milestone{
			TaskID: reqFromTopState.RecvTaskID,
			Kind:   tasks.MilestoneKindNetworkBusy,
			What:   m.topPort().Name(),
		})

		rspMeta := rspI
		removeReqToBottomByID(nextState, rspMeta.RspTo)

		if hook := m.comp.Resources.MigrationHook; hook != nil {
			hook.Completed(reqFromTopState.ReqToBottomAddr)
		}

		m.comp.TaskTracker().EndReqOut(reqFromTopState.SendTaskID)
		m.comp.TaskTracker().EndReqIn(reqFromTopState.RecvTaskID)
	}

	m.bottomPort().RetrieveIncoming()

	return true
}
