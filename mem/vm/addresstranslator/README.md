# addresstranslator — Virtual-to-Physical Request Translator

Package `addresstranslator` provides an address-translating forwarder for the
Akita simulation framework. It sits in the virtual-memory subsystem between a
compute unit and the memory hierarchy: it intercepts `memprotocol.ReadReq` and
`memprotocol.WriteReq` messages carrying virtual addresses, obtains the page translation,
rewrites each request's address to the physical address, and forwards it
downstream.

## How It Works

The translator is driven by three middlewares, run in this order every cycle:
`Ctrl` handles the control commands on the `Control` port, and the two below
do the translation work.

### ParseTranslate — accept and translate

For each incoming `memprotocol.ReadReq` / `memprotocol.WriteReq` on the `Top` port, the
translator computes the virtual page ID (`addr >> Log2PageSize << Log2PageSize`)
and sends a `addrtransprotocol.Req` on the `Translation` port to the provider
resolved by the Spec routing value. The original request's fields are
saved in a transaction record while the translation is outstanding.

### RespondPipeline — forward and respond

1. **parseTranslation** — When a `addrtransprotocol.Rsp` returns, the saved request is
   cloned with its address rewritten to `page.PAddr + offset` (where `offset` is
   the original address modulo the page size) and sent downstream on the `Bottom`
   port to the memory provider resolved by the Spec.Bottom routing value.
2. **respond** — When a `memprotocol.DataReadyRsp` / `memprotocol.WriteDoneRsp` returns on
   `Bottom`, it is matched to the in-flight request and a corresponding response
   is sent back up on `Top` to the original requester.

Up to `NumReqPerCycle` translations and responses are handled each tick.

### Holds, faults, and releases

Page migration stops and restarts the accesses of a translator with
control verbs on the `Control` port (see `mem/CONTROL_PROTOCOL.md`); all of
the related data is in `State`:

- **Hold** (`Addresses`, `PID`) marks pages held. A new access to a held
  page waits in the held list (`State.HeldReqs`) without being translated,
  and an access whose translation arrives after the Hold waits too. The
  held list uses no transaction slot, so accesses to other pages keep
  flowing. The Hold is acknowledged once no issued access to the pages is
  outstanding and no translation of them is in flight. A TLB fills before
  it responds, so after the ack no walk can refill a TLB with the old
  translation.
- **Fault**: an `addrtransprotocol.Rsp` with `Fault` set is not used. The
  access waits in the held list and its page is marked faulted, so the
  later accesses to it wait too, without being translated.
- **Release** (`Addresses`, `PID`) clears held and faulted pages. Their
  waiting accesses move to `State.ReplayReqs` and are sent for translation
  again, in arrival order, before new accesses from `Top`. A fault response
  that is still on its way when the Release arrives faults the page again,
  so a fault handler holds the page before it releases it.

Control is independent of the data path: control commands are processed
while accesses are held or the `Top` port is full. A Drain waits for the
held accesses, so Hold and Release are accepted while draining. A Reset
answers the Holds that are not acknowledged yet with
`memcontrolprotocol.ErrCanceledByReset`.

## Key Types

- `Spec` — immutable configuration: frequency, `Log2PageSize`, `DeviceID`, and
  `NumReqPerCycle`. Port buffer sizes are chosen by the system builder when it
  creates the port instances, not in the `Spec`.
- `State` — mutable runtime data: the control state, the in-flight translation
  transactions, the requests forwarded to the bottom port awaiting
  responses, the held and faulted pages, the Holds not acknowledged yet, and
  the held and replayed accesses.
- `Resources` — empty. Routing uses `mem.AddressMapper` values in Spec (Bottom and Translation).
  Middleware calls their `Find` methods directly; Resources has no routing override.
- `Ports` — the `Top`, `Bottom`, `Translation`, and `Control` ports.
- `Middlewares` — `Ctrl` (control commands), `ParseTranslate` (accept and
  translate), and `RespondPipeline` (forward and respond), run in that order
  every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

```go
spec := addresstranslator.DefaultSpec()
spec.DeviceID = 1
spec.Translation.Destinations = []messaging.RemotePort{"MMU.Top"}
spec.Bottom.Destinations = []messaging.RemotePort{"Cache.Top"}

at := modeling.MakeBuilder[*addresstranslator.Comp](spec).
    WithSimulation(sim).
    Build("AddressTranslator")
at.BindPort("Top", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
    Build())
at.BindPort("Bottom", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
    Build())
at.BindPort("Translation", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
    Build())
at.BindPort("Control", twowaybuffered.MakeBuilder().WithSimulation(sim).Build())
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{})` | Optional empty Resources |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.MakeBuilder`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top**: accepts `memprotocol.ReadReq` / `memprotocol.WriteReq` (virtual addresses), returns
  `memprotocol.DataReadyRsp` / `memprotocol.WriteDoneRsp`.
- **Bottom**: forwards translated `memprotocol.ReadReq` / `memprotocol.WriteReq` (physical
  addresses), receives `memprotocol.DataReadyRsp` / `memprotocol.WriteDoneRsp`.
- **Translation**: sends `addrtransprotocol.Req`, receives `addrtransprotocol.Rsp`.
- **Control**: accepts `memcontrolprotocol.Req` (flush / reset), returns `memcontrolprotocol.Rsp`.
