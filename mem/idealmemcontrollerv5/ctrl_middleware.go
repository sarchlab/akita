package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
)

// ctrlMiddleware handles control messages enable/pause/drain.
type ctrlMiddleware struct { *Comp }

func (m *ctrlMiddleware) Tick() bool {
    made := false
    made = m.handleIncomingCommands() || made
    made = m.handleDrainState() || made
    return made
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
    ctrl := m.tryGetPort("Control")
    if ctrl == nil { return false }
    msg := ctrl.PeekIncoming()
    if msg == nil { return false }

    ctrlMsg := msg.(*mem.ControlMsg)

    // Enable
    if ctrlMsg.Enable {
        m.State.Mode = ModeEnabled
        rsp := ctrlMsg.GenerateRsp()
        if err := ctrl.Send(rsp); err != nil { return false }
        ctrl.RetrieveIncoming()
        return true
    }

    // Drain
    if ctrlMsg.Drain {
        m.State.Mode = ModeDraining
        m.pendingDrainCmd = ctrlMsg
        m.State.DrainPending = true
        ctrl.RetrieveIncoming()
        return true
    }

    // Pause (not enable and not drain)
    m.State.Mode = ModePaused
    rsp := ctrlMsg.GenerateRsp()
    if err := ctrl.Send(rsp); err != nil { return false }
    ctrl.RetrieveIncoming()
    return true
}

func (m *ctrlMiddleware) handleDrainState() bool {
    if m.State.Mode != ModeDraining || !m.State.DrainPending {
        return false
    }
    if len(m.State.Inflight) != 0 {
        return false
    }
    // Now inflight is empty; respond to the pending drain
    if m.pendingDrainCmd == nil {
        // Could happen across snapshot if ControlMsg wasn't persisted.
        // Simply transition to paused.
        m.State.Mode = ModePaused
        m.State.DrainPending = false
        return true
    }
    ctrl := m.tryGetPort("Control")
    if ctrl == nil { return false }
    rsp := m.pendingDrainCmd.GenerateRsp()
    if err := ctrl.Send(rsp); err != nil { return false }
    m.State.Mode = ModePaused
    m.State.DrainPending = false
    m.pendingDrainCmd = nil
    return true
}

// tryGetPort safely tries to get a port by alias without panicking if missing.
func (m *ctrlMiddleware) tryGetPort(name string) sim.Port {
    var p sim.Port
    func() {
        defer func() { _ = recover() }()
        p = m.GetPortByName(name)
    }()
    return p
}
