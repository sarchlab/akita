package addresstranslator

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// parseTranslateMW handles incoming requests from topPort and initiates
// address translation. Control-port handling lives in ctrlMiddleware.
type parseTranslateMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *parseTranslateMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *parseTranslateMW) translationPort() messaging.Port {
	return m.comp.GetPortByName("Translation")
}

// Tick runs translate while the component is enabled. Pause, Drain, and
// Reset all stop new translation work; in-flight transactions continue
// to drain through respondPipelineMW until the component is fully
// paused.
func (m *parseTranslateMW) Tick() bool {
	madeProgress := false

	if m.comp.State.ControlState == memcontrolprotocol.StateEnabled {
		spec := m.comp.Spec()
		for range spec.NumReqPerCycle {
			madeProgress = m.translate() || madeProgress
		}
	}

	return madeProgress
}

func (m *parseTranslateMW) translate() bool {
	if len(m.comp.State.ReplayReqs) > 0 {
		return m.replay()
	}

	itemI := m.topPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(memprotocol.AccessReq)
	nextState := &m.comp.State

	if isPageHeld(nextState, item.GetPID(), item.GetAddress(),
		m.comp.Spec().Log2PageSize) {
		// The page is held or faulted: the access waits for the Release of
		// the page without being translated.
		tracing.TraceReqReceive(m.comp, itemI)
		nextState.HeldReqs = append(nextState.HeldReqs,
			msgToIncomingReqState(itemI))
		nextState.NumHeldReqs++
		m.topPort().RetrieveIncoming()

		return true
	}

	transReq := m.translationReq(item.GetPID(), item.GetAddress())

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

	tracing.TraceReqReceive(m.comp, itemI)
	tracing.TraceReqInitiate(
		m.comp,
		transReq,
		tracing.MsgIDAtReceiver(itemI, m.comp),
	)

	m.addTransaction(incoming, transReq)
	m.topPort().RetrieveIncoming()

	return true
}

// replay translates again the oldest access that a Release let go. If its
// page has been held again since, the access waits again.
func (m *parseTranslateMW) replay() bool {
	nextState := &m.comp.State
	req := nextState.ReplayReqs[0]

	if isPageHeld(nextState, req.PID, req.Address,
		m.comp.Spec().Log2PageSize) {
		nextState.HeldReqs = append(nextState.HeldReqs, req)
		nextState.ReplayReqs = nextState.ReplayReqs[1:]

		return true
	}

	transReq := m.translationReq(req.PID, req.Address)

	if !m.translationPort().CanSend() {
		return false
	}

	m.translationPort().Send(transReq)

	parentID := topTaskID(m.comp, req)
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: parentID,
		Kind:   tracing.MilestoneKindDependency,
		What:   "migration",
	})
	tracing.TraceReqInitiate(m.comp, transReq, parentID)

	m.addTransaction(req, transReq)
	nextState.ReplayReqs = nextState.ReplayReqs[1:]

	return true
}

// translationReq creates the request for the translation of the page of
// vAddr.
func (m *parseTranslateMW) translationReq(
	pid vm.PID,
	vAddr uint64,
) vmprotocol.TranslationReq {
	spec := m.comp.Spec()

	transReq := vmprotocol.TranslationReq{}
	transReq.ID = timing.GetIDGenerator().Generate()
	transReq.Src = m.translationPort().AsRemote()
	transReq.Dst = m.comp.Resources().TranslationProviderMapper.Find(vAddr)
	transReq.PID = pid
	transReq.VAddr = addrToPageID(vAddr, spec.Log2PageSize)
	transReq.DeviceID = spec.DeviceID
	transReq.CanFault = spec.CanFault
	transReq.TrafficClass = "vmprotocol.TranslationReq"

	return transReq
}

func (m *parseTranslateMW) addTransaction(
	incoming incomingReqState,
	transReq vmprotocol.TranslationReq,
) {
	nextState := &m.comp.State
	nextState.Transactions = append(nextState.Transactions, transactionState{
		IncomingReqs:      []incomingReqState{incoming},
		TranslationReqID:  transReq.ID,
		TranslationReqSrc: transReq.Src,
		TranslationReqDst: transReq.Dst,
	})
}
