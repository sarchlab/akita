package idealmemcontrollerv5

import (
    "fmt"
    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
)

// Spec holds immutable configuration values for the controller.
type Spec struct {
    // Core behavior
    Width         int     // Max new reqs consumed per tick
    LatencyCycles int     // Fixed cycles to complete a req
    Freq          sim.Freq

    // Ports
    TopBufSize  int
    CtrlBufSize int

    // Storage
    CapacityBytes uint64 // If Storage is nil, a new storage is created
    UnitSize      uint64 // Optional, 0 to use mem.NewStorage default unit size
}

func (s Spec) Validate() error {
    if s.Width <= 0 {
        return fmt.Errorf("width must be > 0")
    }
    if s.LatencyCycles < 0 {
        return fmt.Errorf("latency cycles must be >= 0")
    }
    if s.Freq <= 0 {
        return fmt.Errorf("freq must be > 0")
    }
    if s.TopBufSize <= 0 || s.CtrlBufSize <= 0 {
        return fmt.Errorf("port buffer sizes must be > 0")
    }
    // CapacityBytes can be 0 if external storage is provided by builder.
    return nil
}

// Defaults returns a Spec with sane defaults.
func Defaults() Spec {
    return Spec{
        Width:         1,
        LatencyCycles: 100,
        Freq:          1 * sim.GHz,
        TopBufSize:    16,
        CtrlBufSize:   4,
        CapacityBytes: 4 * mem.GB,
        UnitSize:      0,
    }
}

