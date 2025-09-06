package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
)

// Ports groups named ports for clarity.
type IO struct {
    Top    sim.Port
    Control sim.Port
}

// Comp is an ideal memory controller with Spec/State/Ports/Middlewares.
type Comp struct {
    *sim.TickingComponent
    sim.MiddlewareHolder

    Spec  Spec
    State State

    // External dependencies
    Storage          *mem.Storage
    AddressConverter mem.AddressConverter

    // Non-serializable transient field to answer drain completion.
    pendingDrainCmd *mem.ControlMsg
}

// Tick delegates to the middleware pipeline.
func (c *Comp) Tick() bool { return c.MiddlewareHolder.Tick() }

// SnapshotState returns a serializable (pure data) snapshot of the state.
// Note: pendingDrainCmd is intentionally excluded.
func (c *Comp) SnapshotState() any {
    return c.State
}

// RestoreState restores the controller state from a snapshot.
func (c *Comp) RestoreState(snapshot any) error {
    if s, ok := snapshot.(State); ok {
        c.State = s
        return nil
    }
    // Support pointer form too
    if sp, ok := snapshot.(*State); ok {
        c.State = *sp
        return nil
    }
    return nil
}
