package addresstranslator

import (
	"log"
	"slices"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type ctrlMiddleware struct {
	comp *Comp
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *ctrlMiddleware) bottomPort() messaging.Port {
	return m.comp.Ports.Bottom
}

func (m *ctrlMiddleware) translationPort() messaging.Port {
	return m.comp.Ports.Translation
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.Ports.Control
}

func (m *ctrlMiddleware) Handle(_ timing.Event) bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles. A Hold is the
	// exception: it does not block the commands that follow it. Control is
	// independent of the data path, so it runs while accesses are held or
	// the Top port is full.
	if m.comp.State.ControlState != controlStateDraining {
		madeProgress = m.handleIncoming() || madeProgress
	}
	madeProgress = m.completePendingHolds() || madeProgress
	return madeProgress
}

// completePendingDrain finalizes a Drain once no transactions, no in-flight
// bottom-side requests, and no held or replayed accesses remain.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != controlStateDraining {
		return false
	}

	if len(state.Transactions) != 0 || len(state.InflightReqToBottom) != 0 ||
		len(state.HeldReqs) != 0 || len(state.ReplayReqs) != 0 {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = controlStatePaused

	return true
}

func (m *ctrlMiddleware) handleIncoming() bool {
	msg, ok := m.ctrlPort().PeekIncoming()
	if !ok {
		return false
	}
	_, ok = msg.Payload.(memcontrolprotocol.Req)
	req := msg
	if !ok {
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Payload.(memcontrolprotocol.Req).Command {
	case memcontrolprotocol.CmdPause:
		return m.handlePause(req)
	case memcontrolprotocol.CmdDrain:
		return m.handleDrain(req)
	case memcontrolprotocol.CmdEnable:
		return m.handleEnable(req)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(req)
	case memcontrolprotocol.CmdHold:
		return m.handleHold(req)
	case memcontrolprotocol.CmdRelease:
		return m.handleRelease(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = controlStatePaused
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = controlStateEnabled
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req messaging.Msg) bool {
	state := &m.comp.State
	state.ControlState = controlStateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleReset(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	m.endInflightTasks()
	state.Transactions = nil
	state.InflightReqToBottom = nil
	state.HeldPages = nil
	state.PendingHolds = nil
	state.HeldReqs = nil
	state.ReplayReqs = nil
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = controlStateEnabled

	for {
		if _, ok := m.topPort().RetrieveIncoming(); !ok {
			break
		}
	}
	for {
		if _, ok := m.bottomPort().RetrieveIncoming(); !ok {
			break
		}
	}
	for {
		if _, ok := m.translationPort().RetrieveIncoming(); !ok {
			break
		}
	}

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// endInflightTasks completes the req_in tracing task of every in-flight request
// and finalizes the req_out it currently owns, so a hard Reset that drops both
// transaction tables leaves no started-never-ended task. A request awaiting translation (Transactions) holds a
// translation req_out (TranslationReqID); one already translated and forwarded
// (InflightReqToBottom) holds the downstream req_out (ReqToBottomID). The end is
// idempotent for a req_out a response already finalized. Mirrors respond()
// (req_in + downstream req_out) and traceTranslationComplete (translation
// req_out) completion.
func (m *ctrlMiddleware) endInflightTasks() {
	for _, trans := range m.comp.State.Transactions {
		for _, inc := range trans.IncomingReqs {
			m.comp.TaskTracker().EndReqIn(inc.RecvTaskID)
		}
		m.comp.TaskTracker().EndReqOut(trans.TranslationTaskID)
	}

	for _, r := range m.comp.State.InflightReqToBottom {
		m.comp.TaskTracker().EndReqIn(r.RecvTaskID)
		m.comp.TaskTracker().EndReqOut(r.SendTaskID)
	}

	for _, r := range m.comp.State.HeldReqs {
		m.comp.TaskTracker().EndReqIn(r.RecvTaskID)
	}

	for _, r := range m.comp.State.ReplayReqs {
		m.comp.TaskTracker().EndReqIn(r.RecvTaskID)
	}
}

// handleHold starts to hold the pages of the request. It does not block the
// Control port: completePendingHolds acknowledges the Hold once no issued
// access to the pages is outstanding and no translation of them is in
// flight.
func (m *ctrlMiddleware) handleHold(msg messaging.Msg) bool {
	req := msg.Payload.(memcontrolprotocol.Req)
	state := &m.comp.State
	log2PageSize := m.comp.Spec.Log2PageSize

	hold := pendingHoldState{ID: msg.ID, Src: msg.Src, PID: req.PID}
	for _, addr := range req.Addresses {
		holdPage(state, req.PID, addr, log2PageSize, false)
		hold.VAddrs = append(hold.VAddrs, addrToPageID(addr, log2PageSize))
	}

	state.PendingHolds = append(state.PendingHolds, hold)
	m.ctrlPort().RetrieveIncoming()

	return true
}

// completePendingHolds acknowledges, in arrival order, the Holds whose pages
// have no access and no translation in flight.
func (m *ctrlMiddleware) completePendingHolds() bool {
	state := &m.comp.State
	madeProgress := false

	for len(state.PendingHolds) > 0 {
		hold := state.PendingHolds[0]
		if m.inFlight(hold) || !m.ctrlPort().CanSend() {
			return madeProgress
		}

		m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdHold,
			hold.Src, hold.ID, true, ""))
		state.PendingHolds = state.PendingHolds[1:]
		madeProgress = true
	}

	return madeProgress
}

// inFlight reports whether an access to a page of the Hold is outstanding or
// a translation of one is in flight.
func (m *ctrlMiddleware) inFlight(hold pendingHoldState) bool {
	state := &m.comp.State
	log2PageSize := m.comp.Spec.Log2PageSize

	onHeldPage := func(pid vm.PID, vAddr uint64) bool {
		if pid != hold.PID {
			return false
		}

		page := addrToPageID(vAddr, log2PageSize)

		return slices.Contains(hold.VAddrs, page)
	}

	for _, t := range state.Transactions {
		for _, r := range t.IncomingReqs {
			if onHeldPage(r.PID, r.Address) {
				return true
			}
		}
	}

	for _, r := range state.InflightReqToBottom {
		if onHeldPage(r.PID, r.VAddr) {
			return true
		}
	}

	return false
}

// handleRelease ends the Hold and the faults of the pages of the request:
// the accesses that wait on them are sent for translation again, in arrival
// order.
func (m *ctrlMiddleware) handleRelease(msg messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	req := msg.Payload.(memcontrolprotocol.Req)
	state := &m.comp.State
	log2PageSize := m.comp.Spec.Log2PageSize

	for _, addr := range req.Addresses {
		m.mustNotBeHolding(req.PID, addrToPageID(addr, log2PageSize))

		if i := findHeldPage(state, req.PID, addr, log2PageSize); i >= 0 {
			state.HeldPages = append(state.HeldPages[:i],
				state.HeldPages[i+1:]...)
		}
	}

	waiting := state.HeldReqs[:0]
	for _, r := range state.HeldReqs {
		if isPageHeld(state, r.PID, r.Address, log2PageSize) {
			waiting = append(waiting, r)
		} else {
			state.ReplayReqs = append(state.ReplayReqs, r)
		}
	}
	state.HeldReqs = waiting

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdRelease,
		msg.Src, msg.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()

	return true
}

// mustNotBeHolding panics if a Hold of the page is not acknowledged yet: a
// page is released only after its Hold completes.
func (m *ctrlMiddleware) mustNotBeHolding(pid vm.PID, page uint64) {
	for _, h := range m.comp.State.PendingHolds {
		if h.PID == pid && slices.Contains(h.VAddrs, page) {
			log.Panicf("%s: release of page %#x before its hold completed",
				m.comp.Name(), page)
		}
	}
}

func (m *ctrlMiddleware) handleUnsupported(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.comp, req.Payload.(memcontrolprotocol.Req).Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	c *Comp,
	cmd memcontrolprotocol.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) messaging.Msg {
	rsp := messaging.Msg{Payload: memcontrolprotocol.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	},
		ID:    c.NewID(),
		Src:   c.Ports.Control.AsRemote(),
		Dst:   dst,
		RspTo: rspTo,
	}

	return rsp
}
