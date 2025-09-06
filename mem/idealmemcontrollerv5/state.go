package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/sim"
)

// Mode indicates the controller's high-level mode.
type Mode int

const (
    ModeEnabled Mode = iota
    ModePaused
    ModeDraining
)

// Txn captures an in-flight read/write in pure data.
type Txn struct {
    IsRead    bool
    Addr      uint64
    Size      uint64   // for reads
    Data      []byte   // for writes
    DirtyMask []bool   // optional for writes
    Remaining int      // countdown in cycles
    Src       sim.RemotePort
    RspTo     string
}

// State is the mutable runtime data of the controller.
type State struct {
    Mode     Mode
    Inflight []Txn

    // If a drain command is pending response when Inflight becomes empty,
    // drainPending is true. We intentionally do not store the original
    // ControlMsg pointer in State to keep State purely serializable.
    DrainPending bool
}

