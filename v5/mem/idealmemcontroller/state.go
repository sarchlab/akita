package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/sim"
)

// mode indicates the controller's high-level mode.
type mode int

const (
    modeEnabled mode = iota
    modePaused
    modeDraining
)

// txn captures an in-flight read/write in pure data.
type txn struct {
    IsRead    bool
    Addr      uint64
    Size      uint64   // for reads
    Data      []byte   // for writes
    DirtyMask []bool   // optional for writes
    Remaining int      // countdown in cycles
    Src       sim.RemotePort
    RspTo     string
}

// state is the mutable runtime data of the controller.
type state struct {
    Mode     mode
    Inflight []txn

    // If a drain command is pending response when Inflight becomes empty,
    // drainPending is true. We intentionally do not store the original
    // ControlMsg pointer in State to keep State purely serializable.
    DrainPending bool
}
