package addresstranslator

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *ctrlMiddleware) translationPort() messaging.Port {
	return m.comp.GetPortByName("Translation")
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles. A Hold is the
	// exception: it does not block the commands that follow it.
	if m.comp.State.ControlState != memcontrolprotocol.StateDraining {
		madeProgress = m.handleIncoming() || madeProgress
	}
	madeProgress = m.completePendingHolds() || madeProgress
	return madeProgress
}

// completePendingDrain finalizes a Drain once no transactions, no in-flight
// bottom-side requests, and no waiting accesses remain.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateDraining {
		return false
	}

	if len(state.Transactions) != 0 || len(state.InflightReqToBottom) != 0 ||
		len(state.HeldReqs) != 0 || len(state.ReplayReqs) != 0 {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = memcontrolprotocol.StatePaused

	return true
}

func (m *ctrlMiddleware) handleIncoming() bool {
	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(memcontrolprotocol.Req)
	if !ok {
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Command {
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

func (m *ctrlMiddleware) handlePause(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = memcontrolprotocol.StatePaused
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = memcontrolprotocol.StateEnabled
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req memcontrolprotocol.Req) bool {
	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleReset(req memcontrolprotocol.Req) bool {
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
	state.ControlState = memcontrolprotocol.StateEnabled

	for m.topPort().RetrieveIncoming() != nil {
	}
	for m.bottomPort().RetrieveIncoming() != nil {
	}
	for m.translationPort().RetrieveIncoming() != nil {
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// endInflightTasks completes the req_in tracing task of every in-flight request
// and finalizes the req_out it currently owns, so a hard Reset that drops both
// transaction tables leaves no started-never-ended task and no leaked
// receiver-registry entry. A request awaiting translation (Transactions) holds a
// translation req_out (TranslationReqID); one already translated and forwarded
// (InflightReqToBottom) holds the downstream req_out (ReqToBottomID). The end is
// idempotent for a req_out a response already finalized. Mirrors respond()
// (req_in + downstream req_out) and traceTranslationComplete (translation
// req_out) completion.
func (m *ctrlMiddleware) endInflightTasks() {
	for _, trans := range m.comp.State.Transactions {
		for _, inc := range trans.IncomingReqs {
			tracing.EndReqInOnReset(m.comp, inc.ID)
		}
		tracing.EndTaskOnReset(m.comp, trans.TranslationReqID)
	}

	for _, r := range m.comp.State.InflightReqToBottom {
		tracing.EndReqInOnReset(m.comp, r.ReqFromTopID)
		tracing.EndTaskOnReset(m.comp, r.ReqToBottomID)
	}

	for _, r := range m.comp.State.HeldReqs {
		tracing.EndReqInOnReset(m.comp, r.ID)
	}

	for _, r := range m.comp.State.ReplayReqs {
		tracing.EndReqInOnReset(m.comp, r.ID)
	}
}

// handleHold starts to hold the pages of the request. It does not block the
// Control port: the Hold is acknowledged by completePendingHolds once no
// access to the pages and no translation of them is in flight.
func (m *ctrlMiddleware) handleHold(req memcontrolprotocol.Req) bool {
	state := &m.comp.State
	log2PageSize := m.comp.Spec().Log2PageSize

	hold := pendingHoldState{ID: req.ID, Src: req.Src, PID: req.PID}
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

		m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdHold,
			hold.Src, hold.ID, true, ""))
		state.PendingHolds = state.PendingHolds[1:]
		madeProgress = true
	}

	return madeProgress
}

// inFlight reports whether an access to a page of the Hold, or a
// translation of one, is in flight.
func (m *ctrlMiddleware) inFlight(hold pendingHoldState) bool {
	state := &m.comp.State
	log2PageSize := m.comp.Spec().Log2PageSize

	onHeldPage := func(pid vm.PID, vAddr uint64) bool {
		if pid != hold.PID {
			return false
		}

		page := addrToPageID(vAddr, log2PageSize)
		for _, p := range hold.VAddrs {
			if p == page {
				return true
			}
		}

		return false
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
// the accesses that wait on them are translated again, in arrival order.
func (m *ctrlMiddleware) handleRelease(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	log2PageSize := m.comp.Spec().Log2PageSize

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

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdRelease,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()

	return true
}

// mustNotBeHolding panics if a Hold of the page is not acknowledged yet: a
// page is released only after its Hold completes.
func (m *ctrlMiddleware) mustNotBeHolding(pid vm.PID, page uint64) {
	for _, h := range m.comp.State.PendingHolds {
		if h.PID != pid {
			continue
		}

		for _, p := range h.VAddrs {
			if p == page {
				log.Panicf("%s: release of page %#x before its hold completed",
					m.comp.Name(), page)
			}
		}
	}
}

func (m *ctrlMiddleware) handleUnsupported(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	port messaging.Port,
	cmd memcontrolprotocol.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) memcontrolprotocol.Rsp {
	rsp := memcontrolprotocol.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "memcontrolprotocol.Rsp"
	return rsp
}
