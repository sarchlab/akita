package dram

import (
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState != control.StateDraining {
		madeProgress = m.handleIncoming() || madeProgress
	}
	return madeProgress
}

// completePendingDrain finalizes a Drain when in-flight transactions
// have settled. "Settled" here means no transactions in the controller
// queue.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != control.StateDraining {
		return false
	}

	if len(state.Transactions) != 0 {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), control.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = control.StatePaused
	return true
}

func (m *ctrlMiddleware) handleIncoming() bool {
	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(control.Req)
	if !ok {
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Command {
	case control.CmdPause:
		return m.handlePause(req)
	case control.CmdDrain:
		return m.handleDrain(req)
	case control.CmdEnable:
		return m.handleEnable(req)
	case control.CmdReset:
		return m.handleReset(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req control.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = control.StatePaused
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), control.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req control.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = control.StateEnabled
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), control.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req control.Req) bool {
	state := &m.comp.State
	state.ControlState = control.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleReset clears runtime state back to a freshly-built controller
// (empty queues, closed bank state). Persistent storage stays
// untouched — it lives in shared resources.
func (m *ctrlMiddleware) handleReset(req control.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	spec := m.comp.Spec()

	state.Transactions = nil
	state.SubTransQueue = subTransQueueState{Entries: []subTransRef{}}
	state.CommandQueues = commandQueueState{
		NumQueues: spec.NumChannel * spec.NumRank,
		Entries:   []queueEntry{},
	}
	state.BankStates = initBankStatesFlat(
		spec.NumRank, spec.NumBankGroup, spec.NumBank)
	state.TickCount = 0
	state.RefreshCycleCounter = 0
	state.RefreshInProgress = false
	state.RefreshCyclesRemaining = 0
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = control.StateEnabled

	resetStatistics(state)

	for m.topPort().RetrieveIncoming() != nil {
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), control.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// resetStatistics zeroes the accumulated DRAM statistics so a Reset returns
// the controller to its freshly-built shape. These counters are reported for
// experiment results, so a reset before a new measurement phase must not carry
// pre-reset traffic into the post-reset run.
func resetStatistics(state *State) {
	state.TotalReadCommands = 0
	state.TotalWriteCommands = 0
	state.TotalActivates = 0
	state.TotalPrecharges = 0
	state.RowBufferHits = 0
	state.RowBufferMisses = 0
	state.TotalCycles = 0
	state.TotalReadLatencyCycles = 0
	state.TotalWriteLatencyCycles = 0
	state.CompletedReads = 0
	state.CompletedWrites = 0
	state.BytesRead = 0
	state.BytesWritten = 0
}

func (m *ctrlMiddleware) handleUnsupported(req control.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, control.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	port messaging.Port,
	cmd control.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) control.Rsp {
	rsp := control.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "control.Rsp"
	return rsp
}
