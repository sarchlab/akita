package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// respondPipelineMW handles translation responses and bottom-port responses.
	"github.com/sarchlab/akita/v5/messaging"
)

type respondPipelineMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *respondPipelineMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *respondPipelineMW) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *respondPipelineMW) translationPort() messaging.Port {
	return m.comp.GetPortByName("Translation")
}

// Tick runs the respond pipeline: respond + parseTranslation. It is
// gated by ControlState — paused agents do not advance in-flight
// transactions; draining and enabled agents do, so a Drain can
// converge.
func (m *respondPipelineMW) Tick() bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}

	madeProgress := false

	spec := m.comp.Spec()

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
	rspI := m.translationPort().PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp := rspI.(vmprotocol.TranslationRsp)
	nextState := &m.comp.State
	transIdx := findTransactionByReqID(nextState.Transactions, rsp.RspTo)

	if transIdx < 0 {
		m.translationPort().RetrieveIncoming()
		return true
	}

	nextTrans := &nextState.Transactions[transIdx]
	reqState := nextTrans.IncomingReqs[0]

	page, proceed := rsp.Page, true
	if hook := m.comp.Resources().MigrationHook; hook != nil {
		page, proceed = hook.Resolve(m.comp, reqState.PID, reqState.Address,
			rsp.Page)
	}

	var translatedReq messaging.Msg
	if proceed {
		translatedReq = m.translatedReq(reqState, page)

		if !m.bottomPort().CanSend() {
			return false
		}
	}

	nextTrans.TranslationDone = true
	nextTrans.Page = rsp.Page

	// Trace before removeTransaction. traceTranslationDone reads the
	// translation identity through nextTrans; removeTransaction's
	// append-shift would leave nextTrans pointing at a different (or no)
	// transaction, finalizing the wrong translation req_out and orphaning
	// the correct one's children.
	m.traceTranslationDone(nextTrans, reqState)

	if proceed {
		m.sendTranslated(reqState, translatedReq)
	} else {
		nextState.HeldReqs = append(nextState.HeldReqs,
			heldReqState{Req: reqState, Page: rsp.Page})
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
	hook := m.comp.Resources().MigrationHook
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

		tracing.AddMilestone(m.comp, tracing.Milestone{
			TaskID: m.topTaskID(held.Req),
			Kind:   tracing.MilestoneKindDependency,
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
	spec := m.comp.Spec()

	return createTranslatedReq(reqState, page,
		spec.Log2PageSize, m.bottomPort().AsRemote(),
		m.comp.Resources().MemProviderMapper)
}

// sendTranslated sends a translated request to the memory system. The caller
// has checked that the Bottom port can send.
func (m *respondPipelineMW) sendTranslated(
	reqState incomingReqState,
	translatedReq messaging.Msg,
) {
	m.bottomPort().Send(translatedReq)

	nextState := &m.comp.State
	reqToBot := buildReqToBottom(reqState, translatedReq)
	nextState.InflightReqToBottom = append(
		nextState.InflightReqToBottom, reqToBot)

	if hook := m.comp.Resources().MigrationHook; hook != nil {
		hook.Issued(reqState.PID, reqState.Address, reqToBot.ReqToBottomAddr)
	}

	topTaskID := m.topTaskID(reqState)

	// The translated request leaves through the Bottom port; this
	// network-busy wait belongs to the top req_in, not to the downstream
	// req_out (which this component sends, so its receiver-side ID would
	// be a phantom task).
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: topTaskID,
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	tracing.TraceReqInitiate(m.comp, translatedReq, topTaskID)
}

func (m *respondPipelineMW) topTaskID(reqState incomingReqState) uint64 {
	fakeFromTop := restoreMemMsg(reqState.ID, reqState.Src, reqState.Dst,
		reqState.RspTo, reqState.Type)

	return tracing.MsgIDAtReceiver(fakeFromTop, m.comp)
}

// traceTranslationDone records, on the top req_in task, the milestone for a
// completed translation and finalizes the translation req_out. trans must
// still be live in State.Transactions — the caller invokes this before
// removeTransaction so the translation identity reads from the correct
// transaction.
func (m *respondPipelineMW) traceTranslationDone(
	trans *transactionState,
	reqState incomingReqState,
) {
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: m.topTaskID(reqState),
		Kind:   tracing.MilestoneKindTranslation,
		What:   "translation",
	})

	fakeTransReq := vmprotocol.TranslationReq{
		MsgMeta: messaging.MsgMeta{
			ID:  trans.TranslationReqID,
			Src: trans.TranslationReqSrc,
			Dst: trans.TranslationReqDst,
		},
	}
	tracing.TraceReqFinalize(m.comp, fakeTransReq)
}

//nolint:funlen,gocyclo
func (m *respondPipelineMW) respond() bool {
	rspI := m.bottomPort().PeekIncoming()
	if rspI == nil {
		return false
	}

	nextState := &m.comp.State

	var (
		reqFromTopState reqToBottomState
		rspToTop        messaging.Msg
	)

	reqInBottom := false

	switch rsp := rspI.(type) {
	case memprotocol.DataReadyRsp:
		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = memprotocol.DataReadyRsp{
				MsgMeta: messaging.MsgMeta{
					ID:           timing.GetIDGenerator().Generate(),
					Src:          m.topPort().AsRemote(),
					Dst:          reqFromTopState.ReqFromTopSrc,
					RspTo:        reqFromTopState.ReqFromTopID,
					TrafficBytes: len(rsp.Data) + 4,
					TrafficClass: "memprotocol.DataReadyRsp",
				},
				Data: rsp.Data,
			}

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				0, reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(m.comp, tracing.Milestone{
				TaskID: tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
				Kind:   tracing.MilestoneKindData,
				What:   "data",
			})
		}
	case memprotocol.WriteDoneRsp:
		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = memprotocol.WriteDoneRsp{
				MsgMeta: messaging.MsgMeta{
					ID:           timing.GetIDGenerator().Generate(),
					Src:          m.topPort().AsRemote(),
					Dst:          reqFromTopState.ReqFromTopSrc,
					RspTo:        reqFromTopState.ReqFromTopID,
					TrafficBytes: 4,
					TrafficClass: "memprotocol.WriteDoneRsp",
				},
			}

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				0, reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(m.comp, tracing.Milestone{
				TaskID: tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
				Kind:   tracing.MilestoneKindSubTask,
				What:   "subtask",
			})
		}
	default:
		log.Panicf("cannot handle respond of type %s", fmt.Sprintf("%T", rspI))
	}

	if reqInBottom {
		if !m.topPort().CanSend() {
			return false
		}

		m.topPort().Send(rspToTop)

		fakeFromTop := restoreMemMsg(
			reqFromTopState.ReqFromTopID,
			reqFromTopState.ReqFromTopSrc,
			reqFromTopState.ReqFromTopDst,
			0, reqFromTopState.ReqFromTopType)

		tracing.AddMilestone(m.comp, tracing.Milestone{
			TaskID: tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
			Kind:   tracing.MilestoneKindNetworkBusy,
			What:   m.topPort().Name(),
		})

		rspMeta := rspI.Meta()
		removeReqToBottomByID(nextState, rspMeta.RspTo)

		if hook := m.comp.Resources().MigrationHook; hook != nil {
			hook.Completed(reqFromTopState.ReqToBottomAddr)
		}

		fakeReqToBottom := restoreMemMsg(
			reqFromTopState.ReqToBottomID,
			reqFromTopState.ReqToBottomSrc,
			reqFromTopState.ReqToBottomDst,
			0, reqFromTopState.ReqToBottomType)
		tracing.TraceReqFinalize(m.comp, fakeReqToBottom)
		tracing.TraceReqComplete(m.comp, fakeFromTop)
	}

	m.bottomPort().RetrieveIncoming()

	return true
}
