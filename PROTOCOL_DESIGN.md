# Protocols as first-class packages

## Motivation

The checkpoint/resume work (PR #144) introduced a codec registry: every message
type that can sit in a port buffer must be registered with
`messaging.RegisterMsg` so a checkpoint can decode it on resume. Registration is
a *manual, separate step*, physically detached from the type definition, with
**no enforcement**. The automated review found four instances of the same
failure mode:

- `mem/vm` translation messages not registered (fixed)
- `mem/datamover` `DataMoveRequest`/`Response` not registered (open)
- `noc/packetization.Flit` not registered (open)
- bare `messaging.MsgMeta` delivered by NoC endpoints not registered (open)

Each is a *latent* bug: `SaveCheckpoint` succeeds, then `LoadCheckpoint` fails
with "unknown message type" only when a checkpoint happens to capture that type
in a buffer. A forgotten registration is invisible until it bites.

The root cause is that **"a message type" and "this type is part of the
checkpointable wire surface" are not connected in the type system.** This design
connects them by making **protocol** a first-class concept.

## Concept

A **protocol** is a named, immutable set of message types that travel over a
port, organized into **roles**. A *protocol package* is a package that defines a
protocol. A **port declares which role(s) of which protocol(s) it speaks.**

Three things fall out of one declaration each:

1. Defining a protocol **registers all its messages** with the checkpoint codec
   (closes the bug class above by construction).
2. A package "is a protocol package" iff it calls `DefineProtocol` (it exports a
   `*Protocol`). No naming convention needed.
3. A port's declared role tells us, and any tooling, exactly what it sends and
   receives — the single discoverable home for "the Top port speaks the mem
   protocol as the responder."

## Roles (per the role-aware decision)

A protocol has named roles. Each role lists the messages it **sends**; what it
**receives** is whatever the *other* roles send. For the common point-to-point
request/response protocol there are two complementary roles:

```
mem protocol
  requester  sends {ReadReq, WriteReq}        receives {DataReadyRsp, WriteDoneRsp}
  responder  sends {DataReadyRsp, WriteDoneRsp} receives {ReadReq, WriteReq}
```

Roles serve documentation and *future* conformance checks. They do **not**
change checkpoint registration: the registered type set is the union of all
roles' sends, so registration coverage is role-agnostic. We model roles now
because they are the natural way to express "the Top port is the responder,"
and because adding them later would be a breaking change to `DeclarePort`.

Single-role protocols are legal and common for symmetric or one-way traffic
(e.g. flits on a link).

## API (in `messaging`)

New public surface — kept deliberately small (opaque handles + one struct):

```go
// A Protocol is a named set of message types with named roles.
type Protocol struct { /* opaque */ }

func (p *Protocol) Name() string
func (p *Protocol) Role(name string) *Role   // panics if undefined
func (p *Protocol) Messages() []Msg          // union of all roles' sends

// A Role is one endpoint's view of a protocol.
type Role struct { /* opaque */ }

func (r *Role) Protocol() *Protocol
func (r *Role) Name() string
func (r *Role) Sends() []Msg

// RoleDef declares a role and the messages it sends.
type RoleDef struct {
    Name  string
    Sends []Msg
}

// DefineProtocol creates a protocol, registers every message type across all
// roles with the checkpoint codec, and returns the handle. Call it as a
// package-level var in the protocol package.
func DefineProtocol(name string, roles ...RoleDef) *Protocol
```

`Role.Receives()` is deliberately **not** part of the initial surface: it only
serves a hypothetical future send-time conformance mode, and adding a getter
later is non-breaking. The role concept itself ships now because `DeclarePort`
needs it (see below).

### DefineProtocol semantics (failure modes are panics, at init time)

- Duplicate protocol name across the module → panic.
- Duplicate role name within a protocol → panic.
- The same concrete message type listed in two roles of one protocol → panic
  ("every message is sent by exactly one role" is an invariant, not an
  assumption).
- A concrete type *may* belong to two different protocols; re-registration with
  the codec is harmless and the audit only needs membership in at least one.

`RegisterMsg` stays as the low-level primitive; `DefineProtocol` becomes the
recommended path and replaces the hand-written `msgcodec.go` files.

### Protocol packages (decision: one package per protocol)

Every protocol lives in its **own, distinctly-named package** that owns the
message types and the protocol definition — a package *is* a protocol:

```go
// mem/memprotocol/protocol.go
package memprotocol

import "github.com/sarchlab/akita/v5/messaging"

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

This one declaration registers every message type listed across the roles.
Adding a new message = define the type + add one entry to a `Sends` list.

The framework's protocol packages:

- `mem/memprotocol` — `ReadReq`/`WriteReq`/`DataReadyRsp`/`WriteDoneRsp` and
  the `AccessReq`/`AccessRsp` interfaces.
- `mem/control` — the control protocol is **separate** from the data protocol
  (control ports are physically distinct ports bound independently of the
  data path). The messages move into the existing control-contract package as
  `control.Req`/`control.Rsp`/`control.Command`, making it a true protocol
  package (protocol + state model + conformance harness).
- `mem/vm/vmprotocol` — `TranslationReq`/`TranslationRsp` (page-table types
  stay in `vm`).
- `mem/datamoverprotocol` — `DataMoveRequest`/`DataMoveResponse`.
- `noc/packetization` — already a pure protocol package; it owns both the
  `link` role (`Flit`) and the `delivery` role (`AssembledMsg`, the
  reassembled product of de-packetization).

## Bare MsgMeta belongs to no protocol (decision)

`MsgMeta` is the envelope, not a message. Sending it bare was only ever done by
the NoC endpoint's traffic-only delivery path, and it is exactly the case that
produced review finding #6 — there is no protocol it could honestly belong to.
That design-level stance is sufficient; there is **no runtime enforcement** (no
`Send`/`Deliver`/registration check):

- `MsgMeta` is simply not part of any protocol, so the coverage audit lists it
  as intentionally unregistered. A bare `MsgMeta` captured in a checkpointed
  buffer would fail at load like any unregistered type.
- The NoC endpoint gains a concrete wrapper type — `AssembledMsg
  {messaging.MsgMeta}` — delivered in place of the bare envelope, registered
  through the packetization protocol's delivery role. Receivers under the
  traffic-only network model only ever consumed `Meta()`, so behavior is
  unchanged. This
  replaces the earlier idea of a "base protocol" inside `messaging`, which
  would have grown `messaging`'s public surface and enshrined the envelope as
  legitimate traffic.

## Port → role binding (requirement: where a port declares its protocol)

`DeclarePort` and `DeclarePortGroup` are extended variadically — backward
compatible, so `DeclarePort("Out")` still compiles and existing callers are
untouched (the `PortOwner` interface is updated to the variadic signature):

```go
func (po *PortOwnerBase) DeclarePort(name string, roles ...*messaging.Role)
func (po *PortOwnerBase) DeclarePortGroup(name string, roles ...*messaging.Role)
```

```go
// mem/idealmemcontroller/builder.go
modelComp.DeclarePort("Top", memprotocol.Responder) // serves read/write
modelComp.DeclarePort("Control", control.Responder)
```

The port owner records `name -> []*Role`, readable through
`PortOwnerBase.PortRoles(name)` (added in Phase 0 so audits and tooling do not
need a later API change). This is the single, discoverable place where "the Top
port speaks the mem protocol as the responder" lives, right next to the
topology declaration — consistent with the externalized-port-creation
convention. A port may speak more than one role (variadic) when it multiplexes
protocols.

A port declared with no role is **untyped/legacy** — still legal during
migration, but visible to tooling via `PortRoles`.

## Enforcement: audit only (per decision), no runtime send check

The bug-class killer is a **registration coverage audit**, a regular `go test`
(it must run under plain `go test ./...`, which CI runs — the acceptance
binaries are separate and stay separate). It has two halves:

1. **Enumerate** (static): use `golang.org/x/tools/go/packages` to load the
   module and find every concrete named type `T` where `T` or `*T` implements
   `messaging.Msg`, defined in a non-test file of an importable (non-`main`)
   package.
2. **Verify** (runtime): assert each found type's wire tag is present in the
   live codec registry, exposed to the audit through a test-only hook
   (`export_test.go`). The audit file blank-imports the packages that define
   messages; the import list is *self-enforcing* — a type whose registration
   never ran fails the registry check with a message telling the author to add
   the blank import and a protocol.

Checking the **registry contents** rather than statically matching
`DefineProtocol` call sites is deliberate: AST-matching composite literals is
brittle (a `Sends` list built through a variable or helper escapes it), and the
registry is the property that actually matters — "decodable at load." It also
tolerates the legacy `RegisterMsg` path during migration. As a special case,
the audit asserts `MsgMeta` itself is **not** registered.

Scope and allowlist policy:

- `_test.go` types are excluded (they exist only inside one test binary).
- `package main` (the `examples/*` binaries) cannot be imported by a test, so
  they are outside the runtime check; the audit logs them as skipped. Examples
  still adopt `DefineProtocol` by convention — they are documentation.
- The allowlist distinguishes **TODO** entries (to burn down to zero in Phase
  2) from **intentional** entries (types that can never appear in a
  checkpointed buffer), so the burn-down has an unambiguous finish line. The
  target at the end of Phase 2 is an empty TODO list.

No `Send`/`Deliver`-time conformance check is added now (the MsgMeta ban above
is a type check on the envelope, not protocol conformance). Roles make an
opt-in conformance mode possible later. The earlier idea of a built-simulation
advisory audit (walk assembled components, report untyped ports) is deferred to
follow-up tooling — `PortRoles` provides everything it needs.

## Codec wire-tag hardening (prerequisite)

The codec tags the wire format with `reflect.Type.String()` — the **short**
package-qualified name (`mem.ReadReq`). Today all Akita package names are
unique so tags are unique, but a protocol-per-package convention invites
same-named packages that would collide in the registry.

Harden the codec tag to the full import path: `PkgPath + "." + Name` (e.g.
`github.com/sarchlab/akita/v5/mem.ReadReq`), preserving the pointer/value
distinction with a `*` prefix for types registered in pointer form — the tag
must be derived identically on the register and encode paths. (One-time
wire-format change; acceptable because checkpoints are already pinned to the
same binary via the build id.)

## Scope of the message surface

- **Application protocols**: `mem` (`mem/memprotocol`), `mem.control`
  (`mem/control`), `vm` (`mem/vm/vmprotocol`), `datamover`
  (`mem/datamoverprotocol`) — each requester/responder.
- **Transport**: `packetization` — single `link` role sending `Flit` (flits are
  symmetric link traffic).
- **Endpoint delivery**: the `packetization.AssembledMsg` wrapper under the
  packetization protocol's `delivery` role (replaces bare `MsgMeta`; see
  above).
- **Test harness**: `noc/acceptance.TrafficMsg` is defined in an importable
  non-test package, so it gets its own protocol — the harness dogfoods the
  convention instead of living on an allowlist.
- **Examples**: `examples/ping`, `examples/tickingping` (importable) and the
  `package main` examples define protocols by convention.
- **Out of scope**: `timing.Event` types. Events are not port traffic; they
  keep the existing `RegisterEvent` path. (A parallel event-coverage audit is a
  possible follow-up but is not part of this design.)

## Migration plan (incremental, not big-bang)

**Phase 0 — Foundation**
- Harden codec wire tag to pointer-aware `PkgPath + Name`; add a tag
  enumeration hook for the audit.
- Add `Protocol`, `Role`, `RoleDef`, `DefineProtocol` to `messaging` with the
  panic semantics above.
- Extend `DeclarePort`/`DeclarePortGroup` to accept `...*Role`; store them;
  add `PortRoles`.
- Forbid bare `MsgMeta` in `Send`/`Deliver`/`RegisterMsg`/`DefineProtocol`.
- Add the registration-coverage audit test (initially with a TODO allowlist so
  it lands green, then burn the TODO list down).

**Phase 1 — Convert the real protocols + close the review findings**
- Replace `mem/msgcodec.go`, `mem/vm/msgcodec.go` with `DefineProtocol`.
- Add protocols for `mem/datamover` (#4), `packetization.Flit` (#5); replace
  the endpoint's bare-`MsgMeta` delivery with `AssembledMsg` (#6). Findings
  #4/#5/#6 close here, structurally.
- Bind ports on the mem components and the noc endpoint/switch to roles; prove
  the checkpoint oracle still passes.

**Phase 2 — Migrate the remaining surface**
- Protocols for `noc/acceptance` and the examples; bind their ports.
- Burn the audit TODO allowlist down to empty.

**Phase 3 — Docs + optional follow-ups**
- Document the protocol convention (`messaging` doc + checkpoint guide).
- (Optional, future) opt-in send-time conformance check derived from roles.
- (Optional, future) built-simulation advisory audit / daisen surfacing of
  port protocols via `PortRoles`.

## Decisions captured

- Role-aware protocols; audit-only enforcement; design note first.
- **Each protocol lives in its own distinctly-named package** that owns the
  message types and the definition (user decision): `memprotocol`,
  `control`, `vmprotocol`, `datamoverprotocol`, `packetization` (which owns
  both the link and delivery roles of the transport).
- Bare `MsgMeta` belongs to no protocol; **no runtime enforcement** (user
  decision) — the endpoint delivers a concrete `AssembledMsg` wrapper, and the
  audit lists `MsgMeta` as intentionally unregistered.
- Audit checks the live registry, not `DefineProtocol` call sites.
- Control is a separate protocol (`mem.control`), not a role pair inside `mem`.
- `RoleDef` struct (not a fluent builder) — matches the declarative one-shot
  shape of the declaration.
- `Role.Receives()` deferred until a conformance mode needs it.
- Untyped ports stay legal during migration; no lint requirement yet.

## Why this is the right call

It converts an unenforceable manual checklist ("remember `msgcodec.go`") into a
structural property derived from declarations the framework wants anyway. The
new public surface is small (opaque `Protocol`/`Role` handles + one `RoleDef`
struct + a variadic `DeclarePort` arg + `PortRoles`), the migration is
incremental and never breaks a green build, and the registration-coverage audit
guarantees the checkpoint bug class can't recur.
