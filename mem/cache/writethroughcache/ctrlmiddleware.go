package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// ctrlMiddleware owns every control verb: the universal verbs (Pause,
// Drain, Enable, Reset) and the conditional verbs (Invalidate, Flush).
// Invalidate and Flush are only legal once the cache is paused or
// drained; issued while Enabled they are rejected with
// ErrMustBePausedOrDrained.
type ctrlMiddleware struct {
	pipeline *pipelineMW
}

// ctrlPort resolves the "Control" port by name. The port instance no longer
// exists at Build time (it is assigned externally), so it is looked up lazily
// on use.
func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.pipeline.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if !m.pipeline.comp.State.IsDraining {
		madeProgress = m.handleIncoming() || madeProgress
	}
	return madeProgress
}

func (m *ctrlMiddleware) completePendingDrain() bool {
	next := &m.pipeline.comp.State
	if !next.IsDraining {
		return false
	}

	for i := range next.Transactions {
		if !next.Transactions[i].Removed {
			return false
		}
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdDrain,
		next.CurrentCmdSrc, next.CurrentCmdID, true, ""))
	next.IsDraining = false
	next.IsPaused = true
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""
	return true
}

func (m *ctrlMiddleware) handleIncoming() bool {
	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(mem.ControlReq)
	if !ok {
		// Drop unexpected message types so the Control port does not stall.
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Command {
	case mem.CmdPause:
		return m.handlePause(req)
	case mem.CmdDrain:
		return m.handleDrain(req)
	case mem.CmdEnable:
		return m.handleEnable(req)
	case mem.CmdReset:
		return m.handleReset(req)
	case mem.CmdInvalidate:
		return m.handleInvalidate(req)
	case mem.CmdFlush:
		return m.handleFlush(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.pipeline.comp.State.IsPaused = true
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req mem.ControlReq) bool {
	next := &m.pipeline.comp.State
	next.IsDraining = true
	// Clear any prior pause so the data pipeline runs and lets in-flight work
	// finish. Intake is gated on IsDraining, so no new traffic is admitted
	// while the drain completes. Without this, a Drain issued from the paused
	// state would never quiesce (the pipeline only runs when !IsPaused) and so
	// would never ack.
	next.IsPaused = false
	next.CurrentCmdID = req.ID
	next.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	next := &m.pipeline.comp.State
	next.IsPaused = false
	next.IsDraining = false

	// Enable resumes from Paused; it must not discard traffic queued while
	// paused, which the pipeline processes once it runs again.
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleReset wipes the cache back to a freshly-built state without
// writeback (the writethrough cache holds no dirty data anyway).
func (m *ctrlMiddleware) handleReset(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	next := &m.pipeline.comp.State
	spec := m.pipeline.comp.Spec()

	next.DirBuf.Clear()
	for i := range next.BankBufs {
		next.BankBufs[i].Clear()
	}
	cache.DirectoryReset(
		&next.DirectoryState, spec.NumSets, spec.WayAssociativity,
		int(1<<spec.Log2BlockSize))
	next.MSHRState = cache.MSHRState{}
	next.Transactions = nil
	next.IsPaused = false
	next.IsDraining = false
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""

	for m.pipeline.topPort().PeekIncoming() != nil {
		m.pipeline.topPort().RetrieveIncoming()
	}
	for m.pipeline.bottomPort().PeekIncoming() != nil {
		m.pipeline.bottomPort().RetrieveIncoming()
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleInvalidate drops directory blocks matching the request's
// address/PID filter (empty address list = all addresses, zero PID = all
// PIDs). The writethrough directory only ever holds clean blocks, so the
// matching blocks are dropped without any writeback. Invalidate is a
// synchronous verb that is only legal once the cache is paused or
// drained; issued while Enabled it is rejected.
func (m *ctrlMiddleware) handleInvalidate(req mem.ControlReq) bool {
	next := &m.pipeline.comp.State
	if !next.IsPaused {
		// Only the fully-paused state is legal; while still draining,
		// in-flight work can still touch the directory after the verb.
		return m.rejectMustBePaused(req)
	}
	if !m.ctrlPort().CanSend() {
		return false
	}

	invalidateBlocks(
		&next.DirectoryState, m.pipeline.comp.Spec(), req.Addresses, req.PID)

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdInvalidate,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleFlush acknowledges a Flush. A writethrough cache never holds
// dirty data (every write is forwarded to the bottom immediately), so
// there is nothing to write back: Flush is a no-op that immediately acks
// Success, leaving the directory intact. Like Invalidate it is only legal
// once the cache is paused or drained.
func (m *ctrlMiddleware) handleFlush(req mem.ControlReq) bool {
	next := &m.pipeline.comp.State
	if !next.IsPaused {
		// Only the fully-paused state is legal; while still draining,
		// in-flight work can still touch the directory after the verb.
		return m.rejectMustBePaused(req)
	}
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdFlush,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// rejectMustBePaused responds that a conditional verb is illegal while
// the cache is Enabled.
func (m *ctrlMiddleware) rejectMustBePaused(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, control.ErrMustBePausedOrDrained))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// invalidateBlocks marks every directory block matching the filter
// invalid. An empty address list matches all addresses; a zero PID
// matches all PIDs. Addresses are aligned to the cache-line size before
// comparison against each block's Tag.
func invalidateBlocks(
	ds *cache.DirectoryState,
	spec Spec,
	addresses []uint64,
	pid vm.PID,
) {
	blockSize := uint64(1) << spec.Log2BlockSize

	matchAddr := make(map[uint64]bool, len(addresses))
	for _, a := range addresses {
		matchAddr[a/blockSize*blockSize] = true
	}

	for si := range ds.Sets {
		set := &ds.Sets[si]
		for wi := range set.Blocks {
			block := &set.Blocks[wi]
			if !block.IsValid {
				continue
			}
			if pid != 0 && vm.PID(block.PID) != pid {
				continue
			}
			if len(addresses) > 0 && !matchAddr[block.Tag] {
				continue
			}
			block.IsValid = false
		}
	}
}

func (m *ctrlMiddleware) handleUnsupported(req mem.ControlReq) bool {
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
	cmd mem.ControlCommand,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) mem.ControlRsp {
	rsp := mem.ControlRsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "mem.ControlRsp"
	return rsp
}
