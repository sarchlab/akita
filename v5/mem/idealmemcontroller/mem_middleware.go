package idealmemcontrollerv5

import (
    "log"
    "reflect"

    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
    "github.com/sarchlab/akita/v4/simv5"
    "github.com/sarchlab/akita/v4/tracing"
)

// memMiddleware handles data-path requests using tick-driven countdown.
type memMiddleware struct {
    *Comp
    sim        *simv5.Simulation
    storageRef string
    stor       Storage
    conv       AddressConverter
}

func (m *memMiddleware) Tick() bool {
    made := false
    made = m.takeNewReqs() || made
    made = m.progressInflight() || made
    return made
}

func (m *memMiddleware) takeNewReqs() bool {
    if m.state.Mode != modeEnabled { // do not take new in PAUSE or DRAIN
        return false
    }
    made := false
    top := m.GetPortByName("Top")
    for i := 0; i < m.Spec.Width; i++ {
        msg := top.RetrieveIncoming()
        if msg == nil {
            break
        }
        switch req := msg.(type) {
        case *mem.ReadReq:
            m.state.Inflight = append(m.state.Inflight, txn{
                IsRead:    true,
                Addr:      m.toInternalAddr(req.Address),
                Size:      req.AccessByteSize,
                Remaining: m.Spec.LatencyCycles,
                Src:       req.Src,
                RspTo:     req.ID,
            })
        case *mem.WriteReq:
            // Clone data to keep Txn pure
            dataCopy := make([]byte, len(req.Data))
            copy(dataCopy, req.Data)
            var maskCopy []bool
            if req.DirtyMask != nil {
                maskCopy = make([]bool, len(req.DirtyMask))
                copy(maskCopy, req.DirtyMask)
            }
            m.state.Inflight = append(m.state.Inflight, txn{
                IsRead:    false,
                Addr:      m.toInternalAddr(req.Address),
                Data:      dataCopy,
                DirtyMask: maskCopy,
                Remaining: m.Spec.LatencyCycles,
                Src:       req.Src,
                RspTo:     req.ID,
            })
        default:
            log.Panicf("idealmemcontrollerv5: unsupported msg %s", reflect.TypeOf(msg))
        }
        tracing.TraceReqReceive(msg, m)
        made = true
    }
    return made
}

func (m *memMiddleware) progressInflight() bool {
    if m.state.Mode == modePaused {
        return false
    }

    top := m.GetPortByName("Top")

    made := m.countdownInflight()

    if len(m.state.Inflight) == 0 {
        // If draining and empty after processing, ctrl middleware will respond.
        return made
    }

    kept, responded := m.respondReady(top)
    if len(kept) != len(m.state.Inflight) {
        m.state.Inflight = kept
    }
    return made || responded
}

// countdownInflight decrements Remaining for all in-flight transactions.
func (m *memMiddleware) countdownInflight() bool {
    progressed := false
    for i := range m.state.Inflight {
        if m.state.Inflight[i].Remaining > 0 {
            m.state.Inflight[i].Remaining--
            progressed = true
        }
    }
    return progressed
}

// respondReady attempts to respond to ready transactions and returns the kept
// transactions that still need processing next ticks, along with whether any
// responses were sent.
func (m *memMiddleware) respondReady(top sim.Port) ([]txn, bool) {
    kept := m.state.Inflight[:0]
    responded := false
    for _, t := range m.state.Inflight {
        if t.Remaining > 0 {
            kept = append(kept, t)
            continue
        }
        if m.handleReadyTxn(top, t) {
            responded = true
            continue
        }
        kept = append(kept, t)
    }
    return kept, responded
}

// handleReadyTxn dispatches to read/write handlers and returns true if a
// response was successfully sent, false if it should be retried next tick.
func (m *memMiddleware) handleReadyTxn(top sim.Port, t txn) bool {
    if t.IsRead {
        return m.sendReadRsp(top, t)
    }
    return m.doWriteAndSendRsp(top, t)
}

func (m *memMiddleware) sendReadRsp(top sim.Port, t txn) bool {
    data, err := m.storage().Read(t.Addr, t.Size)
    if err != nil { log.Panic(err) }
    rsp := mem.DataReadyRspBuilder{}.
        WithSrc(top.AsRemote()).
        WithDst(t.Src).
        WithRspTo(t.RspTo).
        WithData(data).
        Build()
    if err2 := top.Send(rsp); err2 != nil {
        return false
    }
    tracing.TraceReqComplete(rsp, m)
    return true
}

func (m *memMiddleware) doWriteAndSendRsp(top sim.Port, t txn) bool {
    if t.DirtyMask == nil {
        if err := m.storage().Write(t.Addr, t.Data); err != nil { log.Panic(err) }
    } else {
        data, err := m.storage().Read(t.Addr, uint64(len(t.Data)))
        if err != nil { log.Panic(err) }
        for i := 0; i < len(t.Data); i++ {
            if t.DirtyMask[i] { data[i] = t.Data[i] }
        }
        if err := m.storage().Write(t.Addr, data); err != nil { log.Panic(err) }
    }
    rsp := mem.WriteDoneRspBuilder{}.
        WithSrc(top.AsRemote()).
        WithDst(t.Src).
        WithRspTo(t.RspTo).
        Build()
    if err2 := top.Send(rsp); err2 != nil {
        return false
    }
    tracing.TraceReqComplete(rsp, m)
    return true
}

func (m *memMiddleware) toInternalAddr(addr uint64) uint64 {
    if m.conv == nil { return addr }
    return m.conv.ConvertExternalToInternal(addr)
}

// Satisfy sim.Handler for tick events forwarded by TickingComponent.
func (m *memMiddleware) Handle(e sim.Event) error {
    switch e := e.(type) {
    case sim.TickEvent:
        return m.TickingComponent.Handle(e)
    default:
        // ignore
    }
    return nil
}

func (m *memMiddleware) storage() Storage {
    if m.stor != nil { return m.stor }
    if m.sim == nil {
        log.Panic("emu registry not provided; cannot resolve storage")
    }
    v, ok := m.sim.GetState(m.storageRef)
    if !ok {
        log.Panicf("storage ref %q not found in emu registry", m.storageRef)
    }
    s, ok := v.(Storage)
    if !ok {
        log.Panicf("storage ref %q has unexpected type", m.storageRef)
    }
    m.stor = s
    return s
}
