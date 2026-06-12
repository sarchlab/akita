---
sidebar_position: 4
---

# Protocols

The previous pages defined two message types and a port that carries them.
This page names that relationship: a **protocol** is a declared set of
message types that travel over a port, and the port says which protocol it
speaks. One declaration buys you documentation, tooling visibility, and —
most importantly — checkpoint support for every message it carries.

## What You Will Learn

- How to define a protocol with `messaging.DefineProtocol` and **roles**.
- How a port binds to a role in `DeclarePort`.
- The one-package-per-protocol convention used across Akita's libraries.
- What the declaration buys you (checkpointing, the coverage audit).

## Defining a Protocol

A protocol is declared once, as a package-level `var`, in the package that
owns the message types. tickingping's two messages form a symmetric
protocol — both agents send `pingReq` and answer with `pingRsp` on the
same port — so it has a single role:

```go
// pingProtocol is the ping protocol: ping agents are symmetric peers, each
// sending requests and answering with responses on the same port.
var (
    pingProtocol = messaging.DefineProtocol("examples.tickingping",
        messaging.RoleDef{Name: "peer",
            Sends: []messaging.Msg{pingReq{}, pingRsp{}}},
    )
    pingPeer = pingProtocol.Role("peer")
)
```

`DefineProtocol` takes a module-unique protocol name and one `RoleDef` per
role. Each role lists the messages it **sends**; what a role receives is
whatever the protocol's other roles send. Because this runs at package
initialization, defining the protocol also **registers every listed
message type with the checkpoint codec** — that is what makes the messages
serializable when a checkpoint captures them sitting in a port buffer (see
*Writing Checkpointable Code*).

Most protocols are not symmetric. The memory access protocol
(`mem/memprotocol`) is the canonical two-role shape:

```go
var (
    Protocol = messaging.DefineProtocol("mem",
        messaging.RoleDef{Name: "requester",
            Sends: []messaging.Msg{ReadReq{}, WriteReq{}}},
        messaging.RoleDef{Name: "responder",
            Sends: []messaging.Msg{DataReadyRsp{}, WriteDoneRsp{}}},
    )
    Requester = Protocol.Role("requester")
    Responder = Protocol.Role("responder")
)
```

A requester sends reads and writes and receives the responses; a responder
is the mirror image. A cache's `Top` port serves memory requests
(responder), while its `Bottom` port issues them downstream (requester) —
same protocol, opposite roles.

`DefineProtocol` panics at init time on mistakes that would otherwise be
silent: a duplicate protocol name, a duplicate role name, or the same
message type listed in two roles of one protocol.

## Binding Ports to Roles

A port declares the role(s) it speaks right where the component declares
the port — in the builder's `Build`:

```go
comp.DeclarePort("Out", pingPeer)
```

And in a memory component:

```go
modelComp.DeclarePort("Top", memprotocol.Responder)
modelComp.DeclarePort("Bottom", memprotocol.Requester)
modelComp.DeclarePort("Control", memcontrolprotocol.Responder)
```

This is the single discoverable home for "the `Top` port speaks the mem
protocol as the responder." The binding is metadata: it does not change
how messages flow, but it documents the port's contract next to the
topology declaration, and tooling can read it back with
`PortOwnerBase.PortRoles`. A port may bind more than one role when it
multiplexes protocols, and a port with no role is untyped (legal, but
invisible to protocol tooling).

## One Package per Protocol

Across Akita's libraries, every protocol lives in its **own,
distinctly-named package** that owns the message types and the
`DefineProtocol` declaration — a package *is* a protocol:

| Package | Protocol | Roles |
| --- | --- | --- |
| `mem/memprotocol` | memory access | requester / responder |
| `mem/memcontrolprotocol` | memory-agent control | requester / responder |
| `mem/vm/vmprotocol` | address translation | requester / responder |
| `mem/datamoverprotocol` | data move | requester / responder |
| `noc/packetization` | traffic-only transport | link / delivery |

Components import the protocol packages they speak; the protocol package
imports only `messaging` (and whatever its payload types need). When you
write your own component library, give each protocol the same treatment:
a small package with the message types, the `DefineProtocol` var, and the
exported role handles. For self-contained programs (like the examples), an
unexported protocol var in the main package is fine — the declaration
matters, not the visibility.

## What the Declaration Buys

- **Checkpointability.** Every message type in a protocol is registered
  with the checkpoint codec, so a checkpoint that captures it in a port
  buffer can be decoded on resume. Without registration, `LoadCheckpoint`
  fails with `unknown message type`.
- **Audit coverage.** Akita's CI runs a registration-coverage audit that
  finds every concrete `messaging.Msg` type in the module and fails if one
  belongs to no protocol — a forgotten registration is a build break, not
  a latent bug. (The audit covers the Akita module; if you maintain a
  separate library, the runtime registration still works for your types,
  and you can replicate the audit pattern from
  `messaging/protocolaudit_test.go`.)
- **A contract you can read.** The role binding in `DeclarePort` tells the
  next reader exactly what a port sends and receives without tracing the
  middleware code.

One non-message to know about: bare `messaging.MsgMeta` is the envelope
every message embeds, not a message — it belongs to no protocol. Always
define a named message type, even when it carries no payload beyond the
metadata.

## Adding a Message Later

Adding a message to an existing protocol is two steps: define the type,
and add one entry to the right role's `Sends` list. Registration and audit
coverage follow automatically.

## Where to Next

The full design rationale — roles, the audit, the wire format — lives in
the `messaging` package README. For how protocols fit into save/restore,
read *Writing Checkpointable Code*.
