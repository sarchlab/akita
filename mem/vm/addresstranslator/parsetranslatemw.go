package addresstranslator

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/timing"
	"github.com/sarchlab/akita/v5/simulation/tracing"
)

// parseTranslateMW handles incoming requests from topPort and initiates
// address translation. Control-port handling lives in ctrlMiddleware.
type parseTranslateMW struct {
	comp *Comp
}

func (m *parseTranslateMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *parseTranslateMW) translationPort() messaging.Port {
	return m.comp.Ports.Translation
}

// Handle runs translate while the component is enabled. Pause, Drain, and
// Reset all stop new translation work; in-flight transactions continue
// to drain through respondPipelineMW until the component is fully
// paused.
func (m *parseTranslateMW) Handle(_ timing.Event) bool {
	madeProgress := false

	if m.comp.State.ControlState == memcontrolprotocol.StateEnabled {
		spec := m.comp.Spec
		for range spec.NumReqPerCycle {
			madeProgress = m.translate() || madeProgress
		}
	}

	return madeProgress
}

func (m *parseTranslateMW) translate() bool {
	itemI, ok := m.topPort().PeekIncoming()
	if !ok {
		return false
	}

	item := itemI
	vAddr := item.Payload.(memprotocol.AccessReq).GetAddress()
	spec := m.comp.Spec
	vPageID := addrToPageID(vAddr, spec.Log2PageSize)

	transReq := messaging.Msg{Payload: vmprotocol.TranslationReq{
		PID:      item.Payload.(memprotocol.AccessReq).GetPID(),
		VAddr:    vPageID,
		DeviceID: spec.DeviceID},
		ID:  m.comp.NewID(),
		Src: m.translationPort().AsRemote(),
		Dst: m.comp.Resources.TranslationProviderMapper.Find(vAddr),

		TrafficClass: "vmprotocol.TranslationReq"}

	if !m.translationPort().CanSend() {
		return false
	}

	m.translationPort().Send(transReq)

	// The translation request is on its way: the at-head wait to send it on the
	// Translation port — counted on the incoming-buffer task since the request
	// reached the head of the Top buffer — is over. The req_in opened just below
	// starts at retrieve and covers only the processing that follows.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtIncomingBuffer(itemI, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.translationPort().Name(),
	})

	incoming := msgToIncomingReqState(itemI)

	nextState := &m.comp.State

	tracing.TraceReqReceive(m.comp, itemI)
	tracing.TraceReqInitiate(
		m.comp,
		transReq,
		tracing.MsgIDAtReceiver(itemI, m.comp),
	)

	trans := transactionState{
		IncomingReqs:      []incomingReqState{incoming},
		TranslationReqID:  transReq.ID,
		TranslationReqSrc: transReq.Src,
		TranslationReqDst: transReq.Dst,
	}
	nextState.Transactions = append(nextState.Transactions, trans)

	m.topPort().RetrieveIncoming()

	return true
}
