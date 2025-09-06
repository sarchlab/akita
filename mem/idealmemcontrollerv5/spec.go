package idealmemcontrollerv5

import (
    "fmt"
    "github.com/sarchlab/akita/v4/sim"
)

// Spec holds immutable configuration values for the controller.
type Spec struct {
    // Core behavior
    Width         int     // Max new reqs consumed per tick
    LatencyCycles int     // Fixed cycles to complete a req
    Freq          sim.Freq

    // Storage
    StorageRef    string // ID in the EmuStateRegistry
}

func (s Spec) validate() error {
    if s.Width <= 0 {
        return fmt.Errorf("width must be > 0")
    }
    if s.LatencyCycles < 0 {
        return fmt.Errorf("latency cycles must be >= 0")
    }
    if s.Freq <= 0 {
        return fmt.Errorf("freq must be > 0")
    }
    if s.StorageRef == "" {
        return fmt.Errorf("storage ref must be provided")
    }
    return nil
}

// Defaults returns a Spec with sane defaults.
func defaults() Spec {
    return Spec{
        Width:         1,
        LatencyCycles: 100,
        Freq:          1 * sim.GHz,
        StorageRef:    "",
    }
}
