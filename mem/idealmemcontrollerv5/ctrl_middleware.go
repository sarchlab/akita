package idealmemcontrollerv5

import (
    "github.com/sarchlab/akita/v4/mem/mem"
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
    msg := m.IO.Control.PeekIncoming()
    if msg == nil { return false }

    ctrlMsg := msg.(*mem.ControlMsg)

    // Enable
    if ctrlMsg.Enable {
        m.State.Mode = ModeEnabled
        rsp := ctrlMsg.GenerateRsp()
        if err := m.IO.Control.Send(rsp); err != nil { return false }
        m.IO.Control.RetrieveIncoming()
        return true
    }

    // Drain
    if ctrlMsg.Drain {
        m.State.Mode = ModeDraining
        m.pendingDrainCmd = ctrlMsg
        m.State.DrainPending = true
        m.IO.Control.RetrieveIncoming()
        return true
    }

    // Pause (not enable and not drain)
    m.State.Mode = ModePaused
    rsp := ctrlMsg.GenerateRsp()
    if err := m.IO.Control.Send(rsp); err != nil { return false }
    m.IO.Control.RetrieveIncoming()
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
    rsp := m.pendingDrainCmd.GenerateRsp()
    if err := m.IO.Control.Send(rsp); err != nil { return false }
    m.State.Mode = ModePaused
    m.State.DrainPending = false
    m.pendingDrainCmd = nil
    return true
}
