package mmuCache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
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

func (m *ctrlMiddleware) controlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.CurrentState != mmuCacheStateDrain {
		madeProgress = m.handleIncomingCommands() || madeProgress
	}
	return madeProgress
}

// completePendingDrain detects that the data-path middleware finished
// draining (state transitioned to paused) and sends the async Drain
// Rsp.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if !state.PendingDrainRsp {
		return false
	}
	if state.CurrentState != mmuCacheStatePause {
		return false
	}
	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
	msgI := m.controlPort().PeekIncoming()

	if msgI == nil {
		return false
	}

	switch msg := msgI.(type) {
	case memcontrolprotocol.Req:
		return m.handleControlReq(msg)
	default:
		log.Panicf("Unhandled message type: %s", reflect.TypeOf(msgI))
	}

	return false
}

func (m *ctrlMiddleware) handleControlReq(msg memcontrolprotocol.Req) bool {
	switch msg.Command {
	case memcontrolprotocol.CmdEnable:
		return m.performCtrlEnable(msg)
	case memcontrolprotocol.CmdDrain:
		return m.performCtrlDrain(msg)
	case memcontrolprotocol.CmdPause:
		return m.performCtrlPause(msg)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(msg)
	case memcontrolprotocol.CmdInvalidate:
		return m.handleInvalidate(msg)
	case memcontrolprotocol.CmdFlush:
		// An mmuCache caches translations, which are never dirty, so
		// Flush is not meaningful; callers drop entries with Invalidate.
		return m.handleUnsupported(msg)
	default:
		return m.handleUnsupported(msg)
	}
}

func (m *ctrlMiddleware) performCtrlEnable(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.CurrentState = mmuCacheStateEnable

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdEnable,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

func (m *ctrlMiddleware) performCtrlDrain(msg memcontrolprotocol.Req) bool {
	state := &m.comp.State
	state.CurrentState = mmuCacheStateDrain
	state.PendingDrainRsp = true
	state.CurrentCmdID = msg.ID
	state.CurrentCmdSrc = msg.Src

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

func (m *ctrlMiddleware) performCtrlPause(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.CurrentState = mmuCacheStatePause

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdPause,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

// handleInvalidate drops cached translations matching the request's
// address/PID filter (empty address list = all addresses, zero PID = all
// PIDs). Invalidate is a synchronous verb but is only legal once the
// mmuCache is paused or drained; issued while Enabled it is rejected.
func (m *ctrlMiddleware) handleInvalidate(msg memcontrolprotocol.Req) bool {
	state := &m.comp.State
	// Invalidate is only legal once the cache is fully paused; while it is
	// still draining, in-flight responses can still repopulate the table
	// after the invalidate, so accept only the paused state.
	if state.CurrentState != mmuCacheStatePause {
		return m.rejectMustBePaused(msg)
	}
	if !m.controlPort().CanSend() {
		return false
	}

	invalidateEntries(state, m.comp.Spec(), msg.Addresses, msg.PID)

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdInvalidate,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindDependency,
		What:   m.comp.Name() + ".Table",
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

// rejectMustBePaused responds that a conditional verb is illegal while the
// component is Enabled.
func (m *ctrlMiddleware) rejectMustBePaused(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, memcontrolprotocol.ErrMustBePausedOrDrained))
	m.controlPort().RetrieveIncoming()
	return true
}

// invalidateEntries drops every cached page-walk segment matching the
// filter. The mmuCache stores per-level VPN segments rather than full
// virtual addresses, so an address filter is resolved by deriving each
// address's segment at every level and dropping the block that caches it
// (mirroring how lookups and refills derive segments). An empty address
// list drops every segment; a zero PID matches every PID.
func invalidateEntries(
	state *State,
	spec Spec,
	addresses []uint64,
	pid vm.PID,
) {
	if len(addresses) == 0 {
		invalidateAllSegments(state, pid)
		return
	}

	levelWidth := (64 - spec.Log2PageSize) / uint64(spec.NumLevels)
	for _, addr := range addresses {
		vpn := addr >> spec.Log2PageSize
		for level := range state.Table {
			seg := (vpn >> (uint64(level) * levelWidth)) &
				((1 << levelWidth) - 1)
			invalidateSegment(&state.Table[level], pid, seg)
		}
	}
}

// invalidateAllSegments drops every live block whose PID matches the
// filter (zero PID matches all) across all cache levels.
func invalidateAllSegments(state *State, pid vm.PID) {
	for li := range state.Table {
		set := &state.Table[li]
		for wi := range set.Blocks {
			block := set.Blocks[wi]
			if pid != 0 && vm.PID(block.PID) != pid {
				continue
			}
			if _, found := setLookup(set, vm.PID(block.PID), block.Seg); !found {
				continue
			}
			setRemove(set, vm.PID(block.PID), block.Seg)
		}
	}
}

// invalidateSegment drops the block caching (pid, seg) in set if it is
// live, honoring the zero-PID-matches-all rule.
func invalidateSegment(set *setState, pid vm.PID, seg uint64) {
	for wi := range set.Blocks {
		block := set.Blocks[wi]
		if block.Seg != seg {
			continue
		}
		if pid != 0 && vm.PID(block.PID) != pid {
			continue
		}
		if _, found := setLookup(set, vm.PID(block.PID), block.Seg); !found {
			continue
		}
		setRemove(set, vm.PID(block.PID), block.Seg)
	}
}

func (m *ctrlMiddleware) handleReset(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdReset,
		msg.Src, msg.ID, true, ""))
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	state := &m.comp.State
	state.CurrentState = mmuCacheStateEnable
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""

	// A hard reset discards the outstanding bottom requests along with the
	// queued port traffic below. Their (now orphaned) responses must not keep
	// a future Drain from quiescing, nor repopulate the table when they
	// eventually arrive; handleRsp drops responses whose ID is no longer here.
	state.OutstandingBottomReqs = map[uint64]bool{}

	// The dropped walks leave their req_in and forwarded req_out tracing tasks
	// open, and their responses are discarded (OutstandingBottomReqs was cleared
	// above), so end both tasks here — finalize the req_out and complete the
	// req_in — mirroring the normal handleRsp completion so a mid-walk reset
	// leaves no unended tasks rather than just an orphaned registry entry.
	m.endInflightTasks()
	state.InflightReqs = map[uint64]inflightReqState{}

	// Reset is a hard reset: drop the cached page-walk entries so the
	// component matches its freshly-built (empty) table.
	spec := m.comp.Spec()
	state.Table = initSets(spec.NumLevels, spec.NumBlocks)

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}

	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()
	return true
}

// endInflightTasks finalizes the forwarded req_out and completes the original
// req_in for every walk still in flight, so a hard Reset that drops the
// in-flight table leaves no started-never-ended task and no leaked
// receiver-registry entry. The matching responses are discarded by handleRsp
// once OutstandingBottomReqs is cleared, so without this the tasks would never
// close. Mirrors the completion path in handleRsp.
func (m *ctrlMiddleware) endInflightTasks() {
	for _, inflight := range m.comp.State.InflightReqs {
		tracing.EndTaskOnReset(m.comp, inflight.BottomReqID)
		tracing.EndReqInOnReset(m.comp, inflight.TopReqID)
	}
}

func (m *ctrlMiddleware) handleUnsupported(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, memcontrolprotocol.ErrUnsupported))
	m.controlPort().RetrieveIncoming()
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
