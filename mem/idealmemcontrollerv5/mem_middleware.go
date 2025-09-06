package idealmemcontrollerv5

import (
    "log"
    "reflect"

    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
    "github.com/sarchlab/akita/v4/tracing"
)

// memMiddleware handles data-path requests using tick-driven countdown.
type memMiddleware struct { *Comp }

func (m *memMiddleware) Tick() bool {
    made := false
    made = m.takeNewReqs() || made
    made = m.progressInflight() || made
    return made
}

func (m *memMiddleware) takeNewReqs() bool {
    if m.State.Mode != ModeEnabled { // do not take new in PAUSE or DRAIN
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
            m.State.Inflight = append(m.State.Inflight, Txn{
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
            m.State.Inflight = append(m.State.Inflight, Txn{
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
    made := false

    if m.State.Mode == ModePaused {
        return false
    }

    top := m.GetPortByName("Top")

    // Countdown
    for i := range m.State.Inflight {
        if m.State.Inflight[i].Remaining > 0 {
            m.State.Inflight[i].Remaining--
            made = true
        }
    }

    // Respond any ready transactions; rebuild list keeping those not sent
    if len(m.State.Inflight) == 0 {
        // If draining and empty after processing, ctrl middleware will respond.
        return made
    }

    kept := m.State.Inflight[:0]
    for _, t := range m.State.Inflight {
        if t.Remaining > 0 {
            kept = append(kept, t)
            continue
        }

        if t.IsRead {
            // Perform read now
            data, err := m.Storage.Read(t.Addr, t.Size)
            if err != nil { log.Panic(err) }
            rsp := mem.DataReadyRspBuilder{}.
                WithSrc(top.AsRemote()).
                WithDst(t.Src).
                WithRspTo(t.RspTo).
                WithData(data).
                Build()
            if err2 := top.Send(rsp); err2 != nil {
                // Cannot send now; keep it ready to retry next tick
                // Keep Remaining at 0 to retry soon.
                kept = append(kept, t)
                continue
            }
            tracing.TraceReqComplete(rsp, m) // trace completion via rsp
            made = true
        } else {
            // Write
            if t.DirtyMask == nil {
                if err := m.Storage.Write(t.Addr, t.Data); err != nil { log.Panic(err) }
            } else {
                // Read-modify-write
                data, err := m.Storage.Read(t.Addr, uint64(len(t.Data)))
                if err != nil { log.Panic(err) }
                for i := 0; i < len(t.Data); i++ {
                    if t.DirtyMask[i] { data[i] = t.Data[i] }
                }
                if err := m.Storage.Write(t.Addr, data); err != nil { log.Panic(err) }
            }
            rsp := mem.WriteDoneRspBuilder{}.
                WithSrc(top.AsRemote()).
                WithDst(t.Src).
                WithRspTo(t.RspTo).
                Build()
            if err2 := top.Send(rsp); err2 != nil {
                kept = append(kept, t)
                continue
            }
            tracing.TraceReqComplete(rsp, m)
            made = true
        }
    }
    // Update inflight with remaining
    if len(kept) != len(m.State.Inflight) { m.State.Inflight = kept }

    return made
}

func (m *memMiddleware) toInternalAddr(addr uint64) uint64 {
    if m.AddressConverter == nil { return addr }
    return m.AddressConverter.ConvertExternalToInternal(addr)
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
