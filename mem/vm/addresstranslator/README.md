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
and sends a `vmprotocol.TranslationReq` on the `Translation` port to the provider
resolved by the `TranslationProviderMapper`. The original request's fields are
saved in a transaction record while the translation is outstanding.

### RespondPipeline — forward and respond

1. **parseTranslation** — When a `vmprotocol.TranslationRsp` returns, the saved request is
   cloned with its address rewritten to `page.PAddr + offset` (where `offset` is
   the original address modulo the page size) and sent downstream on the `Bottom`
   port to the memory provider resolved by the `MemProviderMapper`.
2. **respond** — When a `memprotocol.DataReadyRsp` / `memprotocol.WriteDoneRsp` returns on
   `Bottom`, it is matched to the in-flight request and a corresponding response
   is sent back up on `Top` to the original requester.

Up to `NumReqPerCycle` translations and responses are handled each tick.

## Key Types

- `Spec` — immutable configuration: frequency, `Log2PageSize`, `DeviceID`, and
  `NumReqPerCycle`. Port buffer sizes are chosen by the system builder when it
  creates the port instances, not in the `Spec`.
- `State` — mutable runtime data: the control state, the in-flight translation
  transactions, and the requests forwarded to the bottom port awaiting
  responses.
- `Resources` — external wiring; holds the `MemProviderMapper` and the
  `TranslationProviderMapper` (both `mem.AddressToPortMapper`).
- `Ports` — the `Top`, `Bottom`, `Translation`, and `Control` ports.
- `Middlewares` — `Ctrl` (control commands), `ParseTranslate` (accept and
  translate), and `RespondPipeline` (forward and respond), run in that order
  every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

```go
spec := addresstranslator.Definition.DefaultSpec
spec.DeviceID = 1

at := addresstranslator.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(addresstranslator.Resources{
        MemProviderMapper:         memMapper,
        TranslationProviderMapper: tlbMapper,
    }).
    WithPorts(addresstranslator.Ports{
        Top:         twowaybuffered.NewPort("AddressTranslator.Top", 4, 4),
        Bottom:      twowaybuffered.NewPort("AddressTranslator.Bottom", 4, 4),
        Translation: twowaybuffered.NewPort("AddressTranslator.Translation", 4, 4),
        Control:     twowaybuffered.NewPort("AddressTranslator.Control", 1, 1),
    }).
    Build("AddressTranslator")
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec` and tweak |
| `WithResources(Resources{...})` | External wiring (memory and translation provider mappers) |
| `WithPorts(Ports{...})` | The port instances, each named `"<instance>.<field>"` (required) |

## Ports

The system builder creates each port with `twowaybuffered.NewPort`, choosing its
buffer sizes, and passes them to `WithPorts`; `Build` binds and registers them.

- **Top**: accepts `memprotocol.ReadReq` / `memprotocol.WriteReq` (virtual addresses), returns
  `memprotocol.DataReadyRsp` / `memprotocol.WriteDoneRsp`.
- **Bottom**: forwards translated `memprotocol.ReadReq` / `memprotocol.WriteReq` (physical
  addresses), receives `memprotocol.DataReadyRsp` / `memprotocol.WriteDoneRsp`.
- **Translation**: sends `vmprotocol.TranslationReq`, receives `vmprotocol.TranslationRsp`.
- **Control**: accepts `memcontrolprotocol.Req` (flush / reset), returns `memcontrolprotocol.Rsp`.
