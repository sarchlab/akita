---
sidebar_position: 9
---

# Protocols

The talking-components tutorial sends messages with nothing more than two
struct types and a port — and that is fully supported. **Messages do not
need a protocol.** Any type embedding `messaging.MsgMeta` can be sent,
received, and type-switched on, exactly as the examples do.

A **protocol** is an opt-in declaration on top of that: a named set of
message types, organized into roles, that travel over a port. You want one
when:

- **Your simulation will be checkpointed.** A message captured in a port
  buffer at save time can only be decoded at load time if its concrete
  type was registered. Defining a protocol registers every message type it
  carries. (Without a protocol, the low-level
  `messaging.RegisterMsg(MyReq{})` in an `init()` does the same for one
  type at a time. If you never checkpoint, neither is needed.)
- **You are building a component library.** A protocol package documents
  the wire contract between your components — what a port sends and
  receives — in one discoverable place, instead of spread across
  middleware code.
- **You want tooling to see your topology's contracts.** Ports bound to
  roles can be read back programmatically (`PortOwnerBase.PortRoles`).

## Defining a Protocol

A protocol is declared once, as a package-level `var`, in the package that
owns the message types:

```go
type MyReq struct {
    messaging.MsgMeta
    Address uint64
}

type MyRsp struct {
    messaging.MsgMeta
}

var (
    Protocol = messaging.DefineProtocol("mypkg",
        messaging.RoleDef{Name: "requester",
            Sends: []messaging.Msg{MyReq{}}},
        messaging.RoleDef{Name: "responder",
            Sends: []messaging.Msg{MyRsp{}}},
    )
    Requester = Protocol.Role("requester")
    Responder = Protocol.Role("responder")
)
```

`DefineProtocol` takes a module-unique protocol name and one `RoleDef` per
**role**. Each role lists the messages it *sends*; what a role receives is
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
    Protocol = messaging.DefineProtocol("packetization",
        messaging.RoleDef{Name: "link",
            Sends: []messaging.Msg{Flit{}}},
        ...
    )
    Link = Protocol.Role("link")
)
```

Because the declaration runs at package initialization, defining the
protocol also **registers every listed message type with the checkpoint
codec** — that is the mechanical payoff.

`DefineProtocol` panics at init time on mistakes that would otherwise be
silent: a duplicate protocol name, a duplicate role name, or the same
message type listed in two roles of one protocol.

## Binding Ports to Roles

A port declares the role(s) it speaks right where the component declares
the port, typically in the builder's `Build`:

```go
modelComp.DeclarePort("Top", memprotocol.Responder)
modelComp.DeclarePort("Bottom", memprotocol.Requester)
modelComp.DeclarePort("Control", memcontrolprotocol.Responder)
```

This is the single discoverable home for "the `Top` port speaks the mem
protocol as the responder." The binding is metadata: it does not change
how messages flow, and there is no runtime conformance check. A port may
bind more than one role when it multiplexes protocols, and a port declared
with no role — like every port in the examples — is untyped and works
exactly the same.

## One Package per Protocol

Across Akita's libraries, every protocol lives in its own,
distinctly-named package that owns the message types and the
`DefineProtocol` declaration — a package *is* a protocol:

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
  `LoadCheckpoint` fails loudly with `unknown message type` — never
  silently. See *Writing Checkpointable Code*.
- **Audit coverage.** Akita's CI runs a registration-coverage audit that
  finds every concrete `messaging.Msg` type in the module's library
  packages and fails if one is unregistered, so a forgotten registration
  is a build break, not a latent bug. The examples are deliberately out of
  the audit's scope — they stay on the simple, protocol-less path. (The
  audit covers the Akita module; for your own library the runtime
  registration works as-is, and you can replicate the audit pattern from
  `messaging/protocolaudit_test.go`.)
- **A contract you can read.** The role binding in `DeclarePort` tells the
  next reader what a port sends and receives without tracing middleware
  code.

One non-message to know about: bare `messaging.MsgMeta` is the envelope
every message embeds, not a message — it belongs to no protocol. Always
define a named message type, even when it carries no payload beyond the
metadata.

## Adding a Message Later

Adding a message to an existing protocol is two steps: define the type,
and add one entry to the right role's `Sends` list. Registration and audit
coverage follow automatically.

## Where to Next

For how protocols fit into save/restore, read *Writing Checkpointable
Code*. The design rationale — roles, the audit, the wire format — lives in
the `messaging` package README.
