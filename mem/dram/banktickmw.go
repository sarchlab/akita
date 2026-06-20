package dram

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/tracing"
)

type bankTickMW struct {
	comp      *modeling.Component[Spec, State, Resources]
	timing    dramTiming
	cmdCycles map[commandKind]int
	ctrl      *controller
}

// Tick advances per-bank timing, issues a command, and refills the command
// queue. Refresh runs in a separate middleware ahead of this one and stalls
// issue via State.RefreshInProgress. Paused DRAM freezes the timing pipeline;
// draining DRAM continues so the drain can converge.
func (m *bankTickMW) Tick() bool {
	next := &m.comp.State
	if next.ControlState == memcontrolprotocol.StatePaused {
		return false
	}
	spec := m.comp.Spec()
	next.TickCount++
	next.TotalCycles++

	// Retire reads/writes whose data/response is now ready (ending their trace
	// tasks), then advance the per-bank timing gaps.
	completed := processPendingCompletions(next)
	m.endSubTransTasks(completed)
	progress := len(completed) > 0
	progress = tickBanks(next) || progress

	// Only issue new commands when refresh (a separate middleware) is not
	// holding the stall flag. While refresh holds the stall, remember that the
	// issue step was blocked so the first command to issue afterward can be
	// charged a refresh milestone for the stall window.
	if !next.RefreshInProgress {
		progress = m.issue(&spec, next) || progress
	} else {
		next.RefreshBlockedIssue = true
	}

	progress = m.ctrl.fillCommandQueue(&spec, next) || progress

	// Keep ticking while reads/writes are still in flight, even on cycles when
	// no timing gap counted down — otherwise a pending completion with no other
	// activity would never be retired.
	if len(next.PendingCompletions) > 0 {
		progress = true
	}

	return progress
}

func (m *bankTickMW) issue(spec *Spec, next *State) bool {
	cmd := m.ctrl.scheduler.Pick(spec, next, &m.timing)
	if cmd == nil {
		return false
	}

	bs := findBankStateByLocation(&next.BankStates, cmd.Location)
	if bs == nil {
		return false
	}

	startCommand(m.cmdCycles, next, bs, cmd)
	updateTiming(m.timing, next, cmd)
	m.traceCmdIssue(next, cmd)

	// This command is the first to issue after a refresh stall window: charge
	// its sub-transaction the refresh wait, then clear the flag so later
	// commands in the same window are not double-charged.
	if next.RefreshBlockedIssue {
		m.traceRefreshStall(next, cmd)
		next.RefreshBlockedIssue = false
	}

	return true
}

// traceRefreshStall records a refresh stall as a hardware_resource milestone on
// the command's sub-transaction trace task. The fake global tRFC stall
// (deviation D2) holds the issue step for tRFC cycles without issuing real
// refresh commands, so without this the refresh window would be invisible in
// the trace; attributing it to the first command that issues afterward charges
// the wait to the sub-transaction that was actually held off.
func (m *bankTickMW) traceRefreshStall(next *State, cmd *commandState) {
	if m.comp.NumHooks() == 0 {
		return
	}
	sub := subTransByRef(next, cmd.SubTransRef)
	if sub == nil {
		return
	}
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: sub.ID,
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   m.comp.Name() + ".refresh",
	})
}

// traceCmdIssue records the command as a milestone on its sub-transaction's
// trace task — each ACT/RD/PRE the controller issues for a sub-transaction is a
// point on that task's timeline. Guarded by NumHooks so the hot path does
// nothing when no tracer is attached.
func (m *bankTickMW) traceCmdIssue(next *State, cmd *commandState) {
	if m.comp.NumHooks() == 0 {
		return
	}
	sub := subTransByRef(next, cmd.SubTransRef)
	if sub == nil {
		return
	}
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: sub.ID,
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   commandKind(cmd.Kind).String(),
	})
}

// endSubTransTasks ends the trace task of every sub-transaction that just
// completed (its data/response became ready).
func (m *bankTickMW) endSubTransTasks(ids []uint64) {
	if m.comp.NumHooks() == 0 {
		return
	}
	for _, id := range ids {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: id})
	}
}
