package dram

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
)

// refreshMiddleware models refresh as a reactive per-cycle behavior. Following
// Akita convention, a controller behavior that runs every cycle and mutates
// State is a Middleware (not a bespoke plugin): the builder adds it ahead of the
// bank-tick middleware, and it communicates with the issue step through
// State.RefreshInProgress (the stall flag).
//
// Today it implements the fake global tRFC stall (deviation D2). Real refresh
// is a different refresh middleware selected by config; the bank-tick issue
// step does not change — it just honors the stall flag.
type refreshMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

// Tick advances the refresh schedule by one cycle. Paused DRAM freezes it, so
// the refresh phase does not drift while the controller is suspended.
func (m *refreshMiddleware) Tick() bool {
	next := &m.comp.State
	if next.ControlState == memcontrolprotocol.StatePaused {
		return false
	}
	spec := m.comp.Spec()
	return runFakeStallRefresh(&spec, next)
}

// runFakeStallRefresh implements periodic refresh scheduling: it stalls command
// issuance for tRFC cycles every tREFI interval by holding State.RefreshInProgress,
// without issuing real refresh commands or closing rows (deviation D2).
func runFakeStallRefresh(spec *Spec, next *State) bool {
	if spec.TREFI <= 0 {
		return false
	}

	// If refresh is in progress, count down.
	if next.RefreshInProgress {
		next.RefreshCyclesRemaining--
		if next.RefreshCyclesRemaining <= 0 {
			next.RefreshInProgress = false
		}
		return true
	}

	// Countdown to next refresh.
	next.RefreshCycleCounter++
	if next.RefreshCycleCounter >= spec.TREFI {
		next.RefreshInProgress = true
		next.RefreshCyclesRemaining = spec.TRFC
		next.RefreshCycleCounter = 0
		return true
	}

	return false
}
