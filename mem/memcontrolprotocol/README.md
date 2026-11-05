# mem/memcontrolprotocol

The protocol package for the uniform memory-agent control protocol. It
defines the protocol (`memcontrolprotocol.Protocol` with requester/responder roles),
its messages and verbs (`memcontrolprotocol.Req`, `memcontrolprotocol.Rsp`, `memcontrolprotocol.Command`),
and conventional error strings. Internal execution states belong to each
component. The conformance harness lives in `mem/internal/memcontroltest`.

See [`../CONTROL_PROTOCOL.md`](../CONTROL_PROTOCOL.md) for the full
protocol reference.

## What this package provides

- **Messages and roles** — `Req`, `Rsp`, `Command`, and requester/responder roles.
- **Error strings** — `ErrUnsupported`, `ErrMustBePausedOrDrained`, and `ErrCanceledByReset`.
- Zero is not a valid `Command`; receivers reject it as unsupported.

Component states are private implementation details. The protocol specifies
observable behavior and response timing, not internal enum values.

## The six verbs

Four are **universal** (every agent supports them); two are
**conditional** (only agents holding private cache-of-memory state).

| Verb       | Class       | Ack   |
| ---------- | ----------- | ----- |
| Pause      | universal   | sync  |
| Drain      | universal   | async |
| Enable     | universal   | sync  |
| Reset      | universal   | sync  |
| Invalidate | conditional | sync  |
| Flush      | conditional | async |

A **sync** verb is acked in the same tick it is received; an **async**
verb (Drain, Flush) is acked when the underlying work completes.

Invalidate and Flush are only legal once the component is paused or
drained. Issued while enabled, they are rejected with
`Success: false, Error: ErrMustBePausedOrDrained`. Unsupported verbs
reply with `Success: false, Error: ErrUnsupported`. A Reset answers the
Holds that are not acknowledged yet with
`Success: false, Error: ErrCanceledByReset`.

## Support matrix

| Matrix                     | Components                                                                               | Extra verbs            |
| -------------------------- | --------------------------------------------------------------------------------------- | ---------------------- |
| `Universal()`              | dram, idealmemcontroller, simplebankedmemory, mmu, gmmu, addresstranslator, rob, datamover | —                      |
| `CacheLike()`              | cache/writeback, cache/writethrough                                                | Invalidate, Flush*     |
| `TranslationCacheLike()`   | vm/tlb, vm/mmuCache                                                                      | Invalidate             |

\* On `writethrough`, Flush is supported but a no-op (no dirty
data); it acks `Success: true` immediately.

## Adding the contract to a component

Akita's built-in component packages use the internal `memcontroltest` package.
Each component package adds one test that builds the component and calls
`RunContract` with its declared matrix:

```go
func TestControlContract(t *testing.T) {
    build := func() *memcontroltest.Harness {
        comp := modeling.MakeBuilder[*Comp](DefaultSpec()).WithSimulation(sim). /* ... */ .Build("MyComp")
        return &memcontroltest.Harness{
            Comp: comp,
            Sim:  sim,
            Ctrl: comp.Ports.Control,
        }
    }

    memcontroltest.RunContract(t, "mycomp", build, memcontroltest.Universal())
}
```

The harness rebuilds the component per verb, delivers each `Req`,
ticks until the `Rsp` arrives (or the tick budget expires), and
checks `Command`, `RspTo`, `Success`, and `Error`. It enforces only the
protocol surface — component-internal effects (directory cleared after
Reset, dirty data written back after Flush, etc.) belong in the
component's own behavior tests.
