package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
)

// Comp is an ideal memory controller with Spec/State/Ports/Middlewares.
type Comp struct {
    *sim.TickingComponent
    sim.MiddlewareHolder

    Spec  Spec
    state state

    // Non-serializable transient field to answer drain completion.
    pendingDrainCmd *mem.ControlMsg
}

// Tick delegates to the middleware pipeline.
func (c *Comp) Tick() bool { return c.MiddlewareHolder.Tick() }

// SnapshotState returns a serializable (pure data) snapshot of the state.
// Note: pendingDrainCmd is intentionally excluded.
func (c *Comp) SnapshotState() any {
    // Deep copy the state to avoid exposing mutable slices to callers.
    snap := state{
        Mode:         c.state.Mode,
        DrainPending: c.state.DrainPending,
    }
    if len(c.state.Inflight) > 0 {
        snap.Inflight = make([]txn, len(c.state.Inflight))
        for i, t := range c.state.Inflight {
            // Copy txn value, then deep copy slices
            nt := t
            if len(t.Data) > 0 {
                nt.Data = append([]byte(nil), t.Data...)
            }
            if len(t.DirtyMask) > 0 {
                nt.DirtyMask = append([]bool(nil), t.DirtyMask...)
            }
            snap.Inflight[i] = nt
        }
    }
    return snap
}

// RestoreState restores the controller state from a snapshot.
func (c *Comp) RestoreState(snapshot any) error {
    switch s := snapshot.(type) {
    case state:
        c.state = s
    case *state:
        c.state = *s
    default:
        // Best effort: ignore unknown shapes silently.
    }
    return nil
}
