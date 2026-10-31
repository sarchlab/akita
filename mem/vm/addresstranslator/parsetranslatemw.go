package addresstranslator

import (
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
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
// paused. While draining, the released accesses are still sent for
// translation again: they were accepted before the Drain, and the Drain
// waits for them.
func (m *parseTranslateMW) Handle(_ timing.Event) bool {
	madeProgress := false

	switch m.comp.State.ControlState {
	case controlStateEnabled:
		for range m.comp.Spec.NumReqPerCycle {
			madeProgress = m.translate() || madeProgress
		}
	case controlStateDraining:
		for range m.comp.Spec.NumReqPerCycle {
			if len(m.comp.State.ReplayReqs) == 0 {
				break
			}

			madeProgress = m.replay() || madeProgress
		}
	}

	return madeProgress
}

func (m *parseTranslateMW) translate() bool {
	if len(m.comp.State.ReplayReqs) > 0 {
		return m.replay()
	}

	itemI, ok := m.topPort().PeekIncoming()
	if !ok {
		return false
	}

	incoming := msgToIncomingReqState(itemI)
	nextState := &m.comp.State

	if isPageHeld(nextState, incoming.PID, incoming.Address,
		m.comp.Spec.Log2PageSize) {
		// The page is held or faulted: the access waits in the held list
		// without being translated.
		incoming.RecvTaskID = m.comp.TaskTracker().StartReqIn(m.comp, itemI)
		nextState.HeldReqs = append(nextState.HeldReqs, incoming)
		nextState.NumHeldReqs++
		m.topPort().RetrieveIncoming()

		return true
	}

	transReq := m.translationReq(incoming)

	if !m.translationPort().CanSend() {
		return false
	}

	// The translation request is on its way: the at-head wait to send it on the
	// Translation port — counted on the incoming-buffer task since the request
	// reached the head of the Top buffer — is over. The req_in opened just below
	// starts at retrieve and covers only the processing that follows.
	m.topPort().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindNetworkBusy, m.translationPort().Name()))

	incoming.RecvTaskID = m.comp.TaskTracker().StartReqIn(m.comp, itemI)
	m.sendTranslation(incoming, transReq)
	m.topPort().RetrieveIncoming()

	return true
}

// replay sends the oldest released access for translation again. If its
// page has been held again since, the access waits again.
func (m *parseTranslateMW) replay() bool {
	nextState := &m.comp.State
	req := nextState.ReplayReqs[0]

	if isPageHeld(nextState, req.PID, req.Address, m.comp.Spec.Log2PageSize) {
		nextState.HeldReqs = append(nextState.HeldReqs, req)
		nextState.ReplayReqs = nextState.ReplayReqs[1:]

		return true
	}

	transReq := m.translationReq(req)

	if !m.translationPort().CanSend() {
		return false
	}

	m.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: req.RecvTaskID,
		Kind:   tasks.MilestoneKindDependency,
		What:   "migration",
	})

	m.sendTranslation(req, transReq)
	nextState.ReplayReqs = nextState.ReplayReqs[1:]

	return true
}

// translationReq creates the request for the translation of the page of an
// access.
func (m *parseTranslateMW) translationReq(incoming incomingReqState) messaging.Msg {
	spec := m.comp.Spec

	return messaging.Msg{Payload: addrtransprotocol.Req{
		PID:      incoming.PID,
		VAddr:    addrToPageID(incoming.Address, spec.Log2PageSize),
		DeviceID: spec.DeviceID},
		ID:  m.comp.NewID(),
		Src: m.translationPort().AsRemote(),
		Dst: spec.Translation.Find(incoming.Address),
	}
}

// sendTranslation sends a translation request and records its transaction.
// The caller has checked that the Translation port can send.
func (m *parseTranslateMW) sendTranslation(
	incoming incomingReqState,
	transReq messaging.Msg,
) {
	translationTaskID := m.comp.TaskTracker().StartReqOut(
		m.comp,
		transReq,
		incoming.RecvTaskID,
	)

	m.translationPort().Send(transReq)

	nextState := &m.comp.State
	nextState.Transactions = append(nextState.Transactions, transactionState{
		TranslationTaskID: translationTaskID,
		IncomingReqs:      []incomingReqState{incoming},
		TranslationReqID:  transReq.ID,
		TranslationReqSrc: transReq.Src,
		TranslationReqDst: transReq.Dst,
	})
}
