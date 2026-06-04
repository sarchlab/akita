package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"
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
	if m.comp.State.ControlState == control.StatePaused {
		return false
	}

	madeProgress := false

	spec := m.comp.Spec()

	for range spec.NumReqPerCycle {
		madeProgress = m.respond() || madeProgress
	}

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

	rsp := rspI.(*vm.TranslationRsp)
	nextState := &m.comp.State
	transIdx := findTransactionByReqID(nextState.Transactions, rsp.RspTo)

	if transIdx < 0 {
		m.translationPort().RetrieveIncoming()
		return true
	}

	nextTrans := &nextState.Transactions[transIdx]
	reqState := nextTrans.IncomingReqs[0]
	spec := m.comp.Spec()
	translatedReq := createTranslatedReq(reqState, rsp.Page,
		spec.Log2PageSize, m.bottomPort().AsRemote(),
		m.comp.Resources().MemProviderMapper)

	if !m.bottomPort().CanSend() {
		return false
	}

	m.bottomPort().Send(translatedReq)

	nextTrans = &nextState.Transactions[transIdx]
	nextTrans.TranslationDone = true
	nextTrans.Page = rsp.Page

	reqToBot := buildReqToBottom(reqState, translatedReq)
	nextState.InflightReqToBottom = append(
		nextState.InflightReqToBottom, reqToBot)
	nextTrans.IncomingReqs = nextTrans.IncomingReqs[1:]

	if len(nextTrans.IncomingReqs) == 0 {
		removeTransaction(nextState, transIdx)
	}

	m.traceTranslationComplete(nextTrans, reqState, translatedReq)

	m.translationPort().RetrieveIncoming()

	return true
}

// traceTranslationComplete records tracing milestones for a completed
// translation and initiates the downstream request trace.
func (m *respondPipelineMW) traceTranslationComplete(
	trans *transactionState,
	reqState incomingReqState,
	translatedReq messaging.Msg,
) {
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(translatedReq, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.bottomPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	fakeFromTop := restoreMemMsg(reqState.ID, reqState.Src, reqState.Dst,
		reqState.RspTo, reqState.SendTaskID, reqState.RecvTaskID, reqState.Type)

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
		tracing.MilestoneKindTranslation,
		"translation",
		m.comp.Name(),
		m.comp,
	)

	fakeTransReq := &vm.TranslationReq{}
	fakeTransReq.ID = trans.TranslationReqID
	fakeTransReq.SendTaskID = trans.TranslationReqSendTaskID
	fakeTransReq.Src = trans.TranslationReqSrc
	fakeTransReq.Dst = trans.TranslationReqDst
	tracing.TraceReqFinalize(fakeTransReq, m.comp)
	tracing.TraceReqInitiate(translatedReq, m.comp,
		tracing.MsgIDAtReceiver(fakeFromTop, m.comp))
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
	case *mem.DataReadyRsp:
		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = &mem.DataReadyRsp{
				Data: rsp.Data,
			}
			rspToTop.Meta().ID = timing.GetIDGenerator().Generate()
			rspToTop.Meta().Src = m.topPort().AsRemote()
			rspToTop.Meta().Dst = reqFromTopState.ReqFromTopSrc
			rspToTop.Meta().RspTo = reqFromTopState.ReqFromTopID
			rspToTop.Meta().TrafficBytes = len(rsp.Data) + 4
			rspToTop.Meta().TrafficClass = "mem.DataReadyRsp"

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				0, reqFromTopState.ReqFromTopSendTaskID,
				reqFromTopState.ReqFromTopRecvTaskID,
				reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
				tracing.MilestoneKindData,
				"data",
				m.comp.Name(),
				m.comp,
			)
		}
	case *mem.WriteDoneRsp:
		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = &mem.WriteDoneRsp{}
			rspToTop.Meta().ID = timing.GetIDGenerator().Generate()
			rspToTop.Meta().Src = m.topPort().AsRemote()
			rspToTop.Meta().Dst = reqFromTopState.ReqFromTopSrc
			rspToTop.Meta().RspTo = reqFromTopState.ReqFromTopID
			rspToTop.Meta().TrafficBytes = 4
			rspToTop.Meta().TrafficClass = "mem.WriteDoneRsp"

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				0, reqFromTopState.ReqFromTopSendTaskID,
				reqFromTopState.ReqFromTopRecvTaskID,
				reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
				tracing.MilestoneKindSubTask,
				"subtask",
				m.comp.Name(),
				m.comp,
			)
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
			0, reqFromTopState.ReqFromTopSendTaskID,
			reqFromTopState.ReqFromTopRecvTaskID,
			reqFromTopState.ReqFromTopType)

		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
			tracing.MilestoneKindNetworkBusy,
			m.topPort().Name(),
			m.comp.Name(),
			m.comp,
		)

		rspMeta := rspI.Meta()
		removeReqToBottomByID(nextState, rspMeta.RspTo)

		fakeReqToBottom := restoreMemMsg(
			reqFromTopState.ReqToBottomID,
			reqFromTopState.ReqToBottomSrc,
			reqFromTopState.ReqToBottomDst,
			0, reqFromTopState.ReqToBottomSendTaskID, 0,
			reqFromTopState.ReqToBottomType)
		tracing.TraceReqFinalize(fakeReqToBottom, m.comp)
		tracing.TraceReqComplete(fakeFromTop, m.comp)
	}

	m.bottomPort().RetrieveIncoming()

	return true
}
