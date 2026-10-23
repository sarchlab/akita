package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
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
	spec := m.comp.Spec
	translatedReq := createTranslatedReq(m.comp.NewID, reqState, rsp.Payload.(addrtransprotocol.Rsp).Page,
		spec.Log2PageSize, m.bottomPort().AsRemote(),
		m.comp.Spec.Bottom)

	if !m.bottomPort().CanSend() {
		return false
	}

	sendTaskID := m.traceTranslationComplete(nextTrans, reqState, translatedReq)
	m.bottomPort().Send(translatedReq)

	nextTrans = &nextState.Transactions[transIdx]
	nextTrans.TranslationDone = true
	nextTrans.Page = rsp.Payload.(addrtransprotocol.Rsp).Page

	reqToBot := buildReqToBottom(reqState, translatedReq)
	reqToBot.SendTaskID = sendTaskID
	nextState.InflightReqToBottom = append(
		nextState.InflightReqToBottom, reqToBot)
	nextTrans.IncomingReqs = nextTrans.IncomingReqs[1:]

	if len(nextTrans.IncomingReqs) == 0 {
		removeTransaction(nextState, transIdx)
	}

	m.translationPort().RetrieveIncoming()

	return true
}

// traceTranslationComplete records, on the top req_in task, the milestones
// for a completed translation (the translation resolving, then the
// network-busy wait to push the translated request downstream), finalizes
// the translation req_out, and initiates the downstream (translated)
// req_out parented to the top req_in. trans must still be live in
// State.Transactions — the caller invokes this before removeTransaction so
// the translation identity reads from the correct transaction.
func (m *respondPipelineMW) traceTranslationComplete(
	trans *transactionState,
	reqState incomingReqState,
	translatedReq messaging.Msg,
) uint64 {
	topTaskID := reqState.RecvTaskID

	m.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: topTaskID,
		Kind:   tasks.MilestoneKindTranslation,
		What:   "translation",
	})

	// The translated request leaves through the Bottom port; this
	// network-busy wait belongs to the top req_in, not to the downstream
	// req_out (which this component sends, so its receiver-side ID would
	// be a phantom task).
	m.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: topTaskID,
		Kind:   tasks.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	m.comp.TaskTracker().EndReqOut(trans.TranslationTaskID)
	return m.comp.TaskTracker().StartReqOut(m.comp, translatedReq, topTaskID)
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

		m.comp.TaskTracker().EndReqOut(reqFromTopState.SendTaskID)
		m.comp.TaskTracker().EndReqIn(reqFromTopState.RecvTaskID)
	}

	m.bottomPort().RetrieveIncoming()

	return true
}
