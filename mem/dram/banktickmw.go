package dram

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
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

	// Retire reads/writes whose data/response is now ready, then advance the
	// per-bank timing gaps.
	progress := processPendingCompletions(next)
	progress = tickBanks(next) || progress

	// Only issue new commands when refresh (a separate middleware) is not
	// holding the stall flag.
	if !next.RefreshInProgress {
		progress = m.issue(&spec, next) || progress
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
	m.fireCmdIssued(cmd, next.TickCount)

	return true
}

// fireCmdIssued notifies observers (counters, energy, tracing) that a command
// was issued, via the Akita hook mechanism. Guarded by NumHooks so the hot path
// allocates nothing when no observer is attached.
func (m *bankTickMW) fireCmdIssued(cmd *commandState, tick uint64) {
	if m.comp.NumHooks() == 0 {
		return
	}
	m.comp.InvokeHook(hooking.HookCtx{
		Domain: m.comp,
		Pos:    HookPosCmdIssued,
		Item:   commandEventFor(cmd, tick),
	})
}
