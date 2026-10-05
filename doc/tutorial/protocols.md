---
sidebar_position: 9
---

# Protocols

A **protocol** is a set of value payload types organized into roles. Every
non-nil `messaging.Msg.Payload` must be registered through a protocol before
`Port.Send` accepts it. Registration also lets JSON restore the concrete type
when a message is checkpointed in a port, component state, or another payload.

Roles document what each endpoint sends. The inspector reads port role tags
without running the simulation.

## Defining a Protocol

A protocol is declared once, as a package-level `var`, in the package that
owns the message types:

```go
type MyReq struct {
    Address uint64
}

type MyRsp struct {
}

var (
    Protocol = messaging.DefineProtocol(
        messaging.RoleDef{Name: "requester",
            Sends: []any{MyReq{}}},
        messaging.RoleDef{Name: "responder",
            Sends: []any{MyRsp{}}},
    )
    Requester = Protocol.Role("requester")
    Responder = Protocol.Role("responder")
)
```

`DefineProtocol` takes one `RoleDef` per **role**. The protocol is named
after the package that calls it: its import path, so two modules can never
define protocols with the same name. Each role lists the messages it *sends*; what a role receives is
whatever the protocol's other roles send. The requester/responder pair
above is the canonical shape — Akita's memory access protocol
(`mem/memprotocol`) looks exactly like this: a requester sends
`ReadReq`/`WriteReq` and receives the responses; a responder is the mirror
image. A cache's `Top` port serves requests (responder) while its `Bottom`
port issues them downstream (requester) — same protocol, opposite roles.

Symmetric traffic gets a single role. Flits on a network link are the
framework example (`noc/packetization`):

```go
var (
    Protocol = messaging.DefineProtocol(
        messaging.RoleDef{Name: "link",
            Sends: []any{Flit{}}},
        ...
    )
    Link = Protocol.Role("link")
)
```

Because the declaration runs at package initialization, defining the
protocol also **registers every listed message type with the checkpoint
codec** — that is the mechanical payoff.

`DefineProtocol` panics at init time on mistakes that would otherwise be
silent: a second protocol in the same package, or an invalid or duplicate
role name. It also rejects pointer payloads and fields containing pointers,
interfaces, channels, functions, or unsafe pointers, even when excluded from JSON.
Nested `messaging.Msg` values are supported. A role name uses only letters, digits, `_`, and `-`. The same
message type may be sent by more than one role, as when a response goes back
to requesters of several kinds.

## Binding Ports to Roles

A port declares the role(s) it speaks right where the component declares
the port: on its field in the component's `Ports` struct, with an
`akita:"role=<protocol>.<role>"` tag. The reorder buffer in `mem/rob`:

```go
type Ports struct {
    // Top receives memory requests and returns their responses in request
    // order.
    Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`

    // Bottom forwards the requests to the bottom unit and receives its
    // responses in any order.
    Bottom messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`

    // Control receives enable, pause, drain, and reset commands.
    Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}
```

`<protocol>` is the import path of the package that defines the protocol,
and `<role>` is the `Name` of one of its `RoleDef`s. A component in another
module, such as a memory model in MGPUSim, names Akita's memory protocol the
same way, `role=github.com/sarchlab/akita/v5/mem/memprotocol.responder`. The
`Ports` struct is the single discoverable home for "the `Top` port speaks
the mem protocol as the responder": the `inspect` package reads the tags
without running the code and reports an error for a tag that names no
defined protocol role.

The binding is metadata: it does not change how messages flow, and there is
no runtime conformance check. A port may bind more than one role when it
multiplexes protocols — list several comma-separated directives,
`akita:"role=example.com/a.x,role=example.com/b.y"` — and a port with no tag — like every port in
the examples — is untyped and works exactly the same.

A port that takes messages of every protocol, such as a message sink that
consumes whatever arrives, speaks `messaging.AnyRole`, the only role of
`messaging.AnyProtocol`:
`akita:"role=github.com/sarchlab/akita/v5/messaging.any"`. A port without a
tag declares nothing; a port with the any role declares that it speaks
anything.
## One Package per Protocol

A package defines at most one protocol, and the protocol is named after it:
the package that owns the message types and the `DefineProtocol`
declaration *is* the protocol. Akita's protocols:

| Package | Protocol | Roles |
| --- | --- | --- |
| `mem/memprotocol` | memory access | requester / responder |
| `mem/memcontrolprotocol` | memory-agent control | requester / responder |
| `mem/vm/vmprotocol` | address translation | requester / responder |
| `mem/datamoverprotocol` | data move | requester / responder |
| `noc/packetization` | traffic-only transport | link / delivery |

Components import the protocol packages they speak; a protocol package
imports only `messaging` (and whatever its payload types need). When you
write your own component library, give each protocol the same treatment: a
small package with the message types, the `DefineProtocol` var, and the
exported role handles.

## What the Declaration Buys

- **Checkpointability.** Every message type in a protocol can be decoded
  when a checkpoint that captured it is loaded. Without registration,
  `LoadCheckpoint` fails loudly with `unknown payload type` — never
  silently. See *Writing Checkpointable Code*.
- **Audit coverage.** CI checks concrete payloads constructed in library message
  literals against the registry. Runtime `Send` also checks every non-nil
  payload, including those supplied dynamically. Examples register their own
  payload types during initialization.
- **A contract you can read.** The role tag on a `Ports` field tells the
  next reader what a port sends and receives without tracing middleware
  code.

A nil payload is metadata-only traffic and needs no registration. Use a distinct
empty payload struct when type identity represents a protocol operation, such as
`WriteDoneRsp{}`. Slice and map storage is shared between sender and receiver;
the message and payload must remain immutable after sending.

## Adding a Message Later

Adding a message to an existing protocol is two steps: define the type,
and add one entry to the right role's `Sends` list. Registration and audit
coverage follow automatically.

## Where to Next

For how protocols fit into save/restore, read *Writing Checkpointable
Code*. The design rationale — roles, the audit, the wire format — lives in
the `messaging` package README.
