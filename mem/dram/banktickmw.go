package dram

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
)

type bankTickMW struct {
	comp      *modeling.Component[Spec, State, Resources]
	timing    dramTiming
	cmdCycles map[commandKind]int
	ctrl      *controller
}

// Tick runs tickBanks, issue, and the command-queue fill. Paused DRAM
// freezes the timing pipeline so in-flight transactions stay where
// they are; draining DRAM continues so the drain can converge.
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

	// Handle periodic refresh
	refreshActive := m.ctrl.refresh.Tick(&spec, next)
	progress = refreshActive || progress

	// Only issue new commands if refresh is not in progress
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

// handleRefresh delegates to the fake-stall refresh routine. It is retained so
// tests can drive refresh scheduling directly; production goes through the
// configured RefreshManager (see controller).
func (*bankTickMW) handleRefresh(spec *Spec, next *State) bool {
	return runFakeStallRefresh(spec, next)
}

// runFakeStallRefresh implements periodic refresh scheduling: it stalls command
// issuance for tRFC cycles every tREFI interval, without issuing real refresh
// commands or closing rows (deviation D2).
func runFakeStallRefresh(spec *Spec, next *State) bool {
	if spec.TREFI <= 0 {
		return false
	}

	// If refresh is in progress, count down
	if next.RefreshInProgress {
		next.RefreshCyclesRemaining--
		if next.RefreshCyclesRemaining <= 0 {
			next.RefreshInProgress = false
		}
		return true
	}

	// Countdown to next refresh
	next.RefreshCycleCounter++
	if next.RefreshCycleCounter >= spec.TREFI {
		next.RefreshInProgress = true
		next.RefreshCyclesRemaining = spec.TRFC
		next.RefreshCycleCounter = 0
		return true
	}

	return false
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
	m.ctrl.onIssue(spec, next, cmd, next.TickCount)

	return true
}
