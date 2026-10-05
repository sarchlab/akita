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
| make a component checkpointable | declare it with a component model — five structs and a `Definition` — and keep all mutable data in `State` (the model does the rest) |
| take / restore a checkpoint | `sim.SaveCheckpoint(path, "")` / `sim.LoadCheckpoint(path, "")` |

There is **no** custom marshalling to write, no wire format to learn, and no
encoder/decoder to call. Default JSON does the work.

## The model: setup rebuilds the shape, the checkpoint restores the runtime

Your system builder — the setup code that assembles the simulation — rebuilds
the *shape*: components, ports, connections, resources, and wiring. The
checkpoint restores only the *runtime* that setup cannot
reproduce: each component's `State`, the messages buffered in ports, shared
resources, the event queue, the engine time, and the ID-generator counter.

That division drives the one rule that matters most:

> **The golden rule: all mutable runtime state lives in `State`.**
>
> Anything that changes during simulation and is not derivable from the restored
> event queue **must** be a field of the component's `State` (or a registered
> resource). Runtime state hidden on a middleware struct — a round-robin cursor, a
> counter, an RNG — is *not* checkpointed, so a resumed run silently diverges.

Keep middleware fields to references that `Build` recreates — usually just the
component pointer; ports live in `Ports` and shared objects in `Resources`. Put
cursors, counters, and in-flight tables in `State`.

## Messages

The recommended way to make message types checkpointable is a **protocol** — a
named set of message types organized into roles (see the *Protocols*
tutorial). Defining the protocol registers every message type it carries with
the checkpoint decoder (which needs the registration because Go cannot
reconstruct a concrete type from a name on its own).

Keep protocol-specific fields in value payload types and register their zero
values in a package-level protocol declaration. Pointer, interface, channel,
function, and unsafe-pointer fields are rejected at any depth, including fields
tagged `json:"-"`. A nested `messaging.Msg` is allowed because it serializes its
own payload. Do not put transient references in a payload.

```go
// in your package
type MyReq struct {
	Address uint64
}

type MyRsp struct {
}

var (
	Protocol = messaging.DefineProtocol( // named "example.com/sim/mypkg"
		messaging.RoleDef{Name: "requester", Sends: []any{MyReq{}}},
		messaging.RoleDef{Name: "responder", Sends: []any{MyRsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)
```

A component state can hold `Buf []messaging.Msg` directly. Plain `encoding/json`
retains each message's routing fields, payload tag, and payload object; nested
messages retain their inner payload types. Nil payloads omit the tag and payload.
No separate wrapper or per-component serialization is needed.

Components then declare which role each port speaks with a tag on the port's
field in their `Ports` struct:

```go
type Ports struct {
	Top messaging.Port `akita:"role=example.com/sim/mypkg.responder"`
}
```

Adding a new message is one type definition plus one entry in a `Sends` list.
A protocol is the only way to register a message type. A registration-coverage
audit in `messaging` checks payload types constructed in library message literals.
`Port.Send` and message JSON encoding also reject unregistered payload types.
Loading a checkpoint whose payload type is no longer registered fails with
`unknown payload type`, naming the full import path and type.

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

Declare the component with one of the component models — `modeling/ticking`,
`modeling/wakeup`, or `modeling/event`. You write **no** checkpoint methods —
the model's generic `Component` already implements them. You only have to put
your data in the right one of the five structs:

| Struct | Holds | Checkpoint treatment |
| --- | --- | --- |
| `Spec` | immutable config (scalars and slices of scalars) | hashed and **compared** on load, not restored |
| `State` | **all** mutable runtime data | serialized and restored — the only component data saved |
| `Resources` | references to shared objects (e.g. `*mem.Storage`) | not serialized; the system builder supplies them again |
| `Ports` | the component's ports | not part of the component's checkpoint; the system builder creates them again, and the simulation saves the messages buffered in them |
| `Middlewares` | the behaviour, holding only references | not serialized; `Build` recreates them with `NewMiddlewares` |

```go
type Spec struct {
	Freq    timing.Freq `json:"freq"`
	Latency int         `json:"latency"`
}

type State struct {
	Inflight    []txn `json:"inflight"`
	NextArbPort int   `json:"next_arb_port"` // a cursor — runtime state, so it lives here
}

type Resources struct {
	Storage *mem.Storage // supplied by the system builder, not serialized
}

type Ports struct {
	Top    messaging.Port
	Bottom messaging.Port
}

type Middlewares struct {
	Pipeline *pipelineMW // holds only its *Comp; its data lives in State
}

type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
```

An event component's pending events are part of the engine's event queue, so
they are saved too — register each event type it schedules (see *Events*
above).

`Spec` may contain only scalars (booleans, numbers, strings, and named types
based on them) and slices or arrays of scalars. No maps or nested structs. `State` is more
permissive: nested structs, slices, arrays, and maps (with string or integer
keys) are all fine. Neither may contain pointers, interfaces, channels, or funcs;
in `State` you may tag a field `json:"-"` to exempt one that setup rebuilds.
Every field of either struct needs a distinct JSON name: `encoding/json`
silently drops colliding fields, so `Build` rejects them.

### The builder validates this for you — loudly

`Definition.Builder()…Build(name)` runs `ValidateSpec`/`ValidateState` and
**panics** at construction if the Spec or State cannot be checkpointed. In
particular, a struct whose state is entirely **unexported** and that has no
`MarshalJSON` serializes as `{}` and would silently lose its contents — the
builder rejects it:

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
and be registered with `sim.RegisterResource`. Components reach such an object
through their `Resources`, and the system builder passes the same instance to
each of them.

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

- `sim/README.md` — the `SaveCheckpoint`/`LoadCheckpoint` orchestration.
- `examples/checkpointdemo` — a runnable save/load demo.
- `mem/acceptancetests/checkpointresume` and
  `mem/acceptancetests/virtualmemcheckpoint` — mid-transaction resume oracles.
