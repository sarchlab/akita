package mmuCache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
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
	madeProgress = m.handleIncomingCommands() || madeProgress
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

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdDrain,
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
	case *mem.ControlReq:
		return m.handleControlReq(msg)
	default:
		log.Panicf("Unhandled message type: %s", reflect.TypeOf(msgI))
	}

	return false
}

func (m *ctrlMiddleware) handleControlReq(msg *mem.ControlReq) bool {
	switch msg.Command {
	case mem.CmdEnable:
		return m.performCtrlEnable(msg)
	case mem.CmdDrain:
		return m.performCtrlDrain(msg)
	case mem.CmdPause:
		return m.performCtrlPause(msg)
	case mem.CmdReset:
		return m.handleReset(msg)
	case mem.CmdInvalidate:
		return m.handleInvalidate(msg)
	case mem.CmdFlush:
		// An mmuCache caches translations, which are never dirty, so
		// Flush is not meaningful; callers drop entries with Invalidate.
		return m.handleUnsupported(msg)
	default:
		return m.handleUnsupported(msg)
	}
}

func (m *ctrlMiddleware) performCtrlEnable(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.CurrentState = mmuCacheStateEnable

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdEnable,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)
	return true
}

func (m *ctrlMiddleware) performCtrlDrain(msg *mem.ControlReq) bool {
	state := &m.comp.State
	state.CurrentState = mmuCacheStateDrain
	state.PendingDrainRsp = true
	state.CurrentCmdID = msg.ID
	state.CurrentCmdSrc = msg.Src

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)
	return true
}

func (m *ctrlMiddleware) performCtrlPause(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.CurrentState = mmuCacheStatePause

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdPause,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)
	return true
}

// handleInvalidate drops cached translations matching the request's
// address/PID filter (empty address list = all addresses, zero PID = all
// PIDs). Invalidate is a synchronous verb but is only legal once the
// mmuCache is paused or drained; issued while Enabled it is rejected.
func (m *ctrlMiddleware) handleInvalidate(msg *mem.ControlReq) bool {
	state := &m.comp.State
	if state.CurrentState == mmuCacheStateEnable {
		return m.rejectMustBePaused(msg)
	}
	if !m.controlPort().CanSend() {
		return false
	}

	invalidateEntries(state, m.comp.Spec(), msg.Addresses, msg.PID)

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdInvalidate,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindDependency,
		m.comp.Name()+".Table",
		m.comp.Name(),
		m.comp,
	)
	return true
}

// rejectMustBePaused responds that a conditional verb is illegal while the
// component is Enabled.
func (m *ctrlMiddleware) rejectMustBePaused(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, control.ErrMustBePausedOrDrained))
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

func (m *ctrlMiddleware) handleReset(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdReset,
		msg.Src, msg.ID, true, ""))
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	state := &m.comp.State
	state.CurrentState = mmuCacheStateEnable
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}

	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, control.ErrUnsupported))
	m.controlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	port messaging.Port,
	cmd mem.ControlCommand,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) *mem.ControlRsp {
	rsp := &mem.ControlRsp{
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
