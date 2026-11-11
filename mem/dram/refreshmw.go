package dram

import "github.com/sarchlab/akita/v5/sim/timing"

// refreshMiddleware models the global tRFC stall (deviation D2). Refresh
// runs before bank issue and keeps its phase through idle intervals.
type refreshMiddleware struct{ comp *Comp }

func startRefreshTimer(c *Comp) {
	if c.Spec.TREFI == 0 || c.Spec.TRFC == 0 {
		return
	}
	c.State.RefreshAt = c.Spec.Freq.NCyclesLater(c.Spec.TREFI, c.CurrentTime())
	c.TickAtBackground(c.State.RefreshAt)
}

// Handle starts or ends a refresh window. The timer alone never reports
// foreground progress; traffic that has to wait for refresh keeps ticking.
func (m *refreshMiddleware) Handle(event timing.Event) bool {
	c := m.comp
	state := &c.State
	if state.ControlState == controlStatePaused || state.RefreshAt == 0 || event.Time() < state.RefreshAt {
		return false
	}
	if state.RefreshInProgress {
		state.RefreshInProgress = false
		// Stay on the tREFI grid, skipping starts inside the current stall. Even
		// a synthetic tRFC >= tREFI gives traffic an issue edge after a stall.
		gap := c.Spec.TREFI - c.Spec.TRFC%c.Spec.TREFI
		state.RefreshAt = c.Spec.Freq.NCyclesLater(gap, state.RefreshAt)
	} else {
		state.RefreshInProgress = true
		state.RefreshAt = c.Spec.Freq.NCyclesLater(c.Spec.TRFC, state.RefreshAt)
	}
	c.TickAtBackground(state.RefreshAt)
	return false
}
