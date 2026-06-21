package gmmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// respondMW handles the bottom→top response path:
	// fetchFromBottom, handleTranslationRsp.
	"github.com/sarchlab/akita/v5/messaging"
)

type respondMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *respondMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *respondMW) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

// Tick runs the respond stage. Paused GMMUs make no progress.
func (m *respondMW) Tick() bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}
	return m.fetchFromBottom()
}

func (m *respondMW) fetchFromBottom() bool {
	rspI := m.bottomPort().PeekIncoming()
	if rspI == nil {
		return false
	}

	if !m.topPort().CanSend() {
		return false
	}

	// The upstream Top port is free: the at-head wait to forward this
	// response on it — counted on the incoming-buffer task since the response
	// reached the head of the Bottom buffer — is over. The req_in opened at
	// retrieve covers only the processing that follows.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtIncomingBuffer(rspI, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.topPort().Name(),
	})

	m.bottomPort().RetrieveIncoming()

	switch rsp := rspI.(type) {
	case vmprotocol.TranslationRsp:
		tracing.TraceReqReceive(m.comp, rsp)
		return m.handleTranslationRsp(rsp)
	default:
		log.Panicf("gmmu cannot handle request of type %s",
			fmt.Sprintf("%T", rspI))
		return false
	}
}

func (m *respondMW) handleTranslationRsp(rsp vmprotocol.TranslationRsp) bool {
	state := &m.comp.State

	reqTransaction, exists := state.RemoteMemReqs[rsp.RspTo]

	if !exists || reqTransaction.ReqID == 0 {
		// Orphaned response: the request it answers was discarded, e.g. by a
		// Reset issued while this remote walk was still outstanding. The
		// message has already been retrieved, so drop it rather than crash.
		return true
	}

	if !m.topPort().CanSend() {
		return false
	}

	rspToTop := vmprotocol.TranslationRsp{
		Page: rsp.Page,
	}
	rspToTop.ID = timing.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort().AsRemote()
	rspToTop.Dst = reqTransaction.ReqSrc
	rspToTop.RspTo = rsp.ID
	rspToTop.TrafficClass = "vmprotocol.TranslationRsp"

	m.topPort().Send(rspToTop)

	// The remote walk is complete. Close the downstream req_out subtask
	// (rsp.RspTo is the downstream request's ID, the key under which the
	// transaction was stored in RemoteMemReqs) and record, on the original
	// req_in, the dependency wait spent while that remote response was in
	// flight. RecvTaskID is the original req_in id; do not key this to the
	// response message.
	tracing.TraceReqFinalize(
		m.comp,
		vmprotocol.TranslationReq{
			MsgMeta: messaging.MsgMeta{ID: rsp.RspTo},
		},
	)
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: reqTransaction.RecvTaskID,
		Kind:   tracing.MilestoneKindTranslation,
		What:   "translation",
	})

	delete(state.RemoteMemReqs, rsp.RspTo)

	return true
}
