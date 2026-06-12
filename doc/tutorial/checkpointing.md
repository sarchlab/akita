---
sidebar_position: 10
---

# Writing Checkpointable Code

Akita can checkpoint a running simulation to a `.tar.gz` archive and resume it
later. The contract is an oracle: *running to the end* must equal *checkpoint,
rebuild the identical simulation, restore, run to the end*.

This guide is for **library users** writing their own messages, events, and
components. The whole user-facing surface is small:

| You want to… | You write |
| --- | --- |
| make messages checkpointable | a protocol: `messaging.DefineProtocol(...)` as a package-level `var` |
| make an event checkpointable | `timing.RegisterEvent(MyEvent{})` in an `init()` |
| make a component checkpointable | split it into `Spec` / `State` / `Resources` (the `modeling.Component` machinery does the rest) |
| take / restore a checkpoint | `sim.SaveCheckpoint(path, "")` / `sim.LoadCheckpoint(path, "")` |

There is **no** custom marshalling to write, no wire format to learn, and no
encoder/decoder to call. Default JSON does the work.

## The model: setup rebuilds the shape, the checkpoint restores the runtime

Your setup code rebuilds the *shape* — components, ports, connections, resources,
and wiring. The checkpoint restores only the *runtime* that setup cannot
reproduce: each component's `State`, the messages buffered in ports, shared
resources, the event queue, the engine time, and the ID-generator counter.

That division drives the one rule that matters most:

> **The golden rule: all mutable runtime state lives in `State`.**
>
> Anything that changes during simulation and is not derivable from the restored
> event queue **must** be a field of the component's `State` (or a registered
> resource). Runtime state hidden on a middleware struct — a round-robin cursor, a
> counter, an RNG — is *not* checkpointed, so a resumed run silently diverges.

Keep middleware fields to structural wiring (ports, downstream references, routing
tables) that setup rebuilds; put cursors, counters, and in-flight tables in
`State`.

## Messages

The recommended way to make message types checkpointable is a **protocol** — a
named set of message types organized into roles (see the *Protocols*
tutorial). Defining the protocol registers every message type it carries with
the checkpoint decoder (which needs the registration because Go cannot
reconstruct a concrete type from a name on its own).

Checklist:

1. Embed `messaging.MsgMeta` and keep every routing/payload field **exported**
   and JSON-serializable. Bare `MsgMeta` is the envelope, not a message — it
   belongs to no protocol.
2. Tag any transient, non-serializable scratch field `json:"-"` (e.g. the
   `Info interface{}` data-plane field).
3. List the type in your package's protocol (a package-level `var` in a
   non-test file, so it runs in production builds).

```go
// in your package
type MyReq struct {
	messaging.MsgMeta
	Address uint64
	Info    interface{} `json:"-"` // transient scratch — excluded
}

type MyRsp struct {
	messaging.MsgMeta
}

var (
	Protocol = messaging.DefineProtocol("mypkg",
		messaging.RoleDef{Name: "requester", Sends: []messaging.Msg{MyReq{}}},
		messaging.RoleDef{Name: "responder", Sends: []messaging.Msg{MyRsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)
```

Components then declare which role each port speaks, right where they declare
the port:

```go
comp.DeclarePort("Top", mypkg.Responder)
```

Adding a new message is one type definition plus one entry in a `Sends` list.
(`messaging.RegisterMsg(MyReq{})` in an `init()` remains as the low-level
primitive, but protocols are the recommended path.) A registration-coverage
audit in `messaging` fails CI for any message type in the Akita module that
belongs to no protocol, and the load itself fails loudly — never silently —
with `unknown message type "yourpkg.MyReq"` if an unregistered message was
captured in a port buffer.

## Events

Identical shape to messages, using the event registry:

1. Embed `timing.EventBase` and keep extra fields exported and JSON-serializable.
2. Register in an `init()`.

```go
type MyEvent struct {
	timing.EventBase
	Payload int
}

func init() {
	timing.RegisterEvent(MyEvent{}) // value or pointer form both work
}
```

A forgotten registration fails at load with `unknown event type "..."`.

## Components

Build on `modeling.Component[Spec, State, Resources]` (or
`EventDrivenComponent`). You write **no** checkpoint methods — the generic
component already implements them. You only have to put your data in the right
one of the three type parameters:

| Type param | Holds | Checkpoint treatment |
| --- | --- | --- |
| `Spec` | immutable config (primitives only) | hashed and **compared** on load, not restored |
| `State` | **all** mutable runtime data | serialized and restored — the only thing saved |
| `Resources` | references to shared objects (e.g. `*mem.Storage`) | not serialized; setup reinjects them |

```go
type Spec struct {
	Freq    timing.Freq `json:"freq"`
	Latency int         `json:"latency"`
}

type State struct {
	Inflight     []txn  `json:"inflight"`
	NextArbPort  int    `json:"next_arb_port"` // a cursor — runtime state, so it lives here
}

type Resources struct {
	Storage *mem.Storage // rebuilt by setup, not serialized
}

type Comp = modeling.Component[Spec, State, Resources]
```

`Spec` must contain only primitives, slices/maps of primitives — no nested
structs. `State` is more permissive: nested structs, slices, and maps (with
string or integer keys) are all fine. Neither may contain pointers, interfaces,
channels, or funcs (tag a field `json:"-"` to exempt one that setup rebuilds).

### The builder validates this for you — loudly

`Build` runs `ValidateSpec`/`ValidateState` and **panics** at construction if the
Spec or State cannot be checkpointed. In particular, a struct whose state is
entirely **unexported** and that has no `MarshalJSON` serializes as `{}` and would
silently lose its contents — the builder rejects it:

```
modeling: component "TLB" has a State that cannot be checkpointed:
  state.lru: type lruset.Set has unexported state but no MarshalJSON, so it
  serializes as {} and would silently lose that state across a checkpoint;
  add MarshalJSON/UnmarshalJSON or export the fields
```

The fix is either to export the fields or to give the type custom
`MarshalJSON`/`UnmarshalJSON` (which the validator then trusts).

## Shared resources

A shared object referenced by several components (memory contents, a page table)
is a registered entity in its own right and implements the `Checkpointable`
interface directly. `mem.Storage` and `vm.PageTable` already do; a custom shared
resource must implement `SaveCheckpoint(io.Writer)` / `LoadCheckpoint(io.Reader)`
and be registered with `sim.RegisterResource`.

## Gotchas

- **Unexported-only structs lose data.** Caught now at `Build` (see above), but
  worth understanding: `encoding/json` ignores unexported fields, so a struct
  made only of them serializes as `{}`. Use exported fields or custom JSON.
- **Runtime state on a middleware diverges silently.** It is not in `State`, so it
  is not saved. This is *not* caught automatically — follow the golden rule.
- **Serial engine only.** `SaveCheckpoint`/`LoadCheckpoint` reject a
  `ParallelEngine`.
- **Run with tracing off for a deterministic resume.** The tracing task-ID side
  table consumes the global ID generator, perturbing the checkpointed ID counter.
- **Take mid-transaction checkpoints with `SerialEngine.RunUntil(t)`**, which
  stops at a reproducible boundary (every event with time ≤ `t`), unlike `Run`
  (drains everything) or `Pause` (stops at a non-reproducible point).

## Testing your types

You usually do not need a dedicated test: a message type that belongs to no
protocol fails the registration-coverage audit in `messaging` at CI time, a
forgotten `RegisterEvent` fails loudly when a checkpoint that captured the
event is loaded, and a non-serializable `State` panics at `Build`. For
end-to-end confidence, take and
restore a checkpoint in a test and assert the resumed run matches an
uninterrupted one — see the resume oracles under `mem/acceptancetests` for the
pattern.

## See also

- `simulation/README.md` — the `SaveCheckpoint`/`LoadCheckpoint` orchestration.
- `examples/checkpointdemo` — a runnable save/load demo.
- `mem/acceptancetests/checkpointresume` and
  `mem/acceptancetests/virtualmemcheckpoint` — mid-transaction resume oracles.
