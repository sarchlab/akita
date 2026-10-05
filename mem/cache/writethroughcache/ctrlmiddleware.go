package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/timing"
	"github.com/sarchlab/akita/v5/simulation/tracing"
)

// ctrlMiddleware owns every control verb: the universal verbs (Pause,
// Drain, Enable, Reset) and the conditional verbs (Invalidate, Flush).
// Invalidate and Flush are only legal once the cache is paused or
// drained; issued while Enabled they are rejected with
// ErrMustBePausedOrDrained.
type ctrlMiddleware struct {
	comp *Comp
}

// ctrlPort returns the Control port.
func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.Ports.Control
}

// Handle completes a pending Drain and handles the next control command.
func (m *ctrlMiddleware) Handle(_ timing.Event) bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if !m.comp.State.IsDraining {
		madeProgress = m.handleIncoming() || madeProgress
	}
	return madeProgress
}

func (m *ctrlMiddleware) completePendingDrain() bool {
	next := &m.comp.State
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

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdDrain,
		next.CurrentCmdSrc, next.CurrentCmdID, true, ""))
	next.IsDraining = false
	next.IsPaused = true
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""
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
		// Drop unexpected message types so the Control port does not stall.
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
	case memcontrolprotocol.CmdInvalidate:
		return m.handleInvalidate(req)
	case memcontrolprotocol.CmdFlush:
		return m.handleFlush(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.IsPaused = true
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req messaging.Msg) bool {
	next := &m.comp.State
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

func (m *ctrlMiddleware) handleEnable(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	next := &m.comp.State
	next.IsPaused = false
	next.IsDraining = false

	// Enable resumes from Paused; it must not discard traffic queued while
	// paused, which the pipeline processes once it runs again.
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleReset wipes the cache back to a freshly-built state without
// writeback (the writethrough cache holds no dirty data anyway).
func (m *ctrlMiddleware) handleReset(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	next := &m.comp.State
	spec := m.comp.Spec

	next.DirBuf.Clear()
	for i := range next.BankBufs {
		next.BankBufs[i].Clear()
	}
	cache.DirectoryReset(
		&next.DirectoryState, spec.numSets(), spec.WayAssociativity,
		spec.blockSize())
	next.MSHRState = cache.MSHRState{}
	m.endInflightTasks()
	next.Transactions = nil
	next.IsPaused = false
	next.IsDraining = false
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""

	for {
		if _, ok := m.comp.Ports.Top.PeekIncoming(); !ok {
			break
		}
		m.comp.Ports.Top.RetrieveIncoming()
	}
	for {
		if _, ok := m.comp.Ports.Bottom.PeekIncoming(); !ok {
			break
		}
		m.comp.Ports.Bottom.RetrieveIncoming()
	}

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// endInflightTasks completes the req_in tracing task of every in-flight
// transaction, finalizes its downstream read/write req_out tasks, and closes
// its directory-pipeline and bank subtasks, so a hard Reset that drops the
// transaction table leaves no started-never-ended task and no leaked
// receiver-registry entry. A slot already marked Removed can still hold a
// downstream write whose response will be ignored, so every slot is visited and
// the already-ended req_in end is idempotent. Mirrors respondStage (req_in),
// bottomparser (req_out), and the directory/bank pipelines (subtasks).
func (m *ctrlMiddleware) endInflightTasks() {
	comp := m.comp

	for i := range comp.State.Transactions {
		trans := &comp.State.Transactions[i]

		switch {
		case trans.HasRead:
			tracing.EndReqInOnReset(comp, trans.ReadMeta.ID)
		case trans.HasWrite:
			tracing.EndReqInOnReset(comp, trans.WriteMeta.ID)
		}

		if trans.HasReadToBottom {
			tracing.EndTaskOnReset(comp, trans.ReadToBottomMeta.ID)
		}

		if trans.HasWriteToBottom {
			tracing.EndTaskOnReset(comp, trans.WriteToBottomMeta.ID)
		}

		if trans.DirPipelineTaskID != 0 {
			tracing.EndTaskOnReset(comp, trans.DirPipelineTaskID)
		}

		if trans.BankTaskID != 0 {
			tracing.EndTaskOnReset(comp, trans.BankTaskID)
		}
	}
}

// handleInvalidate drops directory blocks matching the request's
// address/PID filter (empty address list = all addresses, zero PID = all
// PIDs). The writethrough directory only ever holds clean blocks, so the
// matching blocks are dropped without any writeback. Invalidate is a
// synchronous verb that is only legal once the cache is paused or
// drained; issued while Enabled it is rejected.
func (m *ctrlMiddleware) handleInvalidate(req messaging.Msg) bool {
	request := req.Payload.(memcontrolprotocol.Req)

	next := &m.comp.State
	if !next.IsPaused {
		// Only the fully-paused state is legal; while still draining,
		// in-flight work can still touch the directory after the verb.
		return m.rejectMustBePaused(req)
	}
	if !m.ctrlPort().CanSend() {
		return false
	}

	invalidateBlocks(
		&next.DirectoryState, m.comp.Spec, request.Addresses, request.PID)

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdInvalidate,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleFlush acknowledges a Flush. A writethrough cache never holds
// dirty data (every write is forwarded to the bottom immediately), so
// there is nothing to write back: Flush is a no-op that immediately acks
// Success, leaving the directory intact. Like Invalidate it is only legal
// once the cache is paused or drained.
func (m *ctrlMiddleware) handleFlush(req messaging.Msg) bool {
	next := &m.comp.State
	if !next.IsPaused {
		// Only the fully-paused state is legal; while still draining,
		// in-flight work can still touch the directory after the verb.
		return m.rejectMustBePaused(req)
	}
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdFlush,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// rejectMustBePaused responds that a conditional verb is illegal while
// the cache is Enabled.
func (m *ctrlMiddleware) rejectMustBePaused(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.comp, req.Payload.(memcontrolprotocol.Req).Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrMustBePausedOrDrained))
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
		ID:           c.NewID(),
		Src:          c.Ports.Control.AsRemote(),
		Dst:          dst,
		RspTo:        rspTo,
		TrafficClass: "memcontrolprotocol.Rsp"}

	return rsp
}
