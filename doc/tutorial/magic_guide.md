---
sidebar_position: 8
---

# How to Enable "Magic" in Akita V5

This guide is for **researchers** who need a simulator to do something
non-standard: an idealized TLB that always hits, an infinite cache, a global
counter that ticks on every access, a fault injected at cycle N, a custom
structure two components share. These experiments often *feel* like they
require breaking a component's encapsulation — reaching in and poking its
guts.

They don't. Akita gives you a small set of **explicit seams** that cover
essentially every "magic" experiment, while keeping the simulator correct,
reproducible, and reviewable. This guide is the catalog of those seams and a
set of worked recipes.

> **Reference code locations:**
>
> | Concept | Path | What it gives you |
> |---|---|---|
> | Component models | `v5/modeling/ticking/` (and `wakeup/`, `event/`) | the five structs, `Definition.Builder()`, `WithResources` |
> | Hooks | `v5/hooking/` | `Hook`, `HookCtx`, `AcceptHook` |
> | Port hook points | `v5/messaging/port.go` | `HookPosPortMsgSend`, `HookPosPortMsgRecvd` |
> | Buffer hook points | `v5/queueing/buffer.go` | `HookPosBufPush`, `HookPosBufPop` |
> | Ideal component example | `v5/mem/idealmemcontroller/` | A "perfect memory" variant |
> | Middleware | `v5/modeling/middleware.go` | `Middleware` (`Handle(e timing.Event) bool`) |
> | Writing a component | the [Create a Component](/tutorial/components/what_is_a_component) tutorial | The full component how-to |

---

## 1. The Philosophy: Explicit Seams, Not a Backdoor

Two principles run through Akita's design:

1. **Wiring happens at setup, by injection.** A component never looks another
   component up by name at runtime. The system builder constructs everything
   and hands each component direct references to what it needs, through its
   `Resources` and `Ports`.
2. **Runtime state is encapsulated.** A component's `State` is written only by
   the component's own code; other components reach it only through functions
   the owner's package chose to expose.

"Magic" is enabled by working *with* these principles, not around them. There
is deliberately **no global `GetByName(...)` backdoor** that lets one component
reach into another at runtime, because such a backdoor:

- requires the target's fields to be **public** (encapsulation gone),
- does a **string lookup + type assertion** on every access (slow, unsafe),
- **hides** the dependency from anyone reading the code, and
- makes **timing-incorrect** synchronous cross-component mutation the path of
  least resistance — the single most common way to silently corrupt a
  discrete-event model.

The seams below give you everything the backdoor would, with none of those
costs.

> **The escape hatch is always open.** Go's visibility is *package-scoped*. If
> a seam genuinely doesn't exist for your experiment, you can always **fork the
> component's package** — inside `tlb`, every field of the TLB is visible. You
> have the source. Encapsulation only stops one component from reaching into
> *another's* guts in the shared codebase; it never stops you from editing the
> component you're studying.

---

## 2. The Decision Guide

Before reaching for a seam, answer three questions about your "magic":

1. **Is it pure instrumentation** (you only *observe*, or accumulate a
   statistic that doesn't change what the simulator does)?
   → **Hooks** (§3.3), optionally writing into an **injected shared resource**
   (§3.4).

2. **Does it change a component's *behavior*** (always hit, never evict, a
   novel policy)?
   → **A variant component** (§3.1), a **`Spec` flag** (§3.2), or a **custom
   middleware** (§3.5).

3. **Does it affect *modeled timing*** (the experiment must consume cycles, or
   one component must influence another's schedule)?
   → **Messages between components**, not a direct call. A direct cross-component
   mutation is instantaneous and bypasses the event model — correct only for
   instrumentation that never feeds back into timing.

| Your intent | Seam |
|---|---|
| Replace a component wholesale | Variant component (§3.1) |
| Tweak a knob | `Spec` config (§3.2) |
| Count / log / trace an event | Hook (§3.3) |
| Share a structure across components | Inject a `Resources` reference (§3.4) |
| Add cross-cutting per-tick behavior | Middleware (§3.5) |
| Warm up / preload state | Seed `State` at setup (§3.6) |
| Something genuinely unforeseen | Fork the package (§3.7) |

---

## 3. The Seven Seams

### 3.1 Swap the Component (Variant)

When the magic is "this whole component behaves differently," write an
alternative that satisfies the **same port interface** and substitute it at
build time. Nothing else in the system needs to know.

This is already how Akita ships idealizations: [`idealmemcontroller`](/packages/mem/idealmemcontroller/)
is "perfect memory" living next to the real `dram` and `simplebankedmemory`
models. To run with perfect memory, you build an `idealmemcontroller` where you
would have built `dram`.

```go
top := messaging.NewPort("DRAM.Top", 1024, 1024)
control := messaging.NewPort("DRAM.Control", 4, 4)

// Real run:
ctrl := dram.Definition.Builder().
    WithSimulation(sim).
    WithResources(dram.Resources{Storage: storage}).
    WithPorts(dram.Ports{Top: top, Control: control}).
    Build("DRAM")

// Idealized run — same ports, drop-in:
ctrl := idealmemcontroller.Definition.Builder().
    WithSimulation(sim).
    WithResources(idealmemcontroller.Resources{Storage: storage}).
    WithPorts(idealmemcontroller.Ports{Top: top, Control: control}).
    Build("DRAM")
```

Both components declare the same `Ports` fields (`Top`, `Control`) with the
same protocol roles, so everything wired to `ctrl.Ports.Top` is unchanged.

Use this for: perfect TLB, ideal memory, a magic interconnect, an oracle
predictor — anything where "the component is fundamentally different."

### 3.2 Configure via `Spec`

If the magic is a *knob*, it belongs in the component's `Spec` (immutable
build-time configuration). Capacities, latencies, and policy flags are `Spec`
fields. Setting `latency = 0` or `capacity = 1<<30` at build time is not a
hack — it is the intended way to reach an extreme operating point.

```go
spec := tlb.Definition.DefaultSpec
spec.NumSets = 1
spec.NumWays = 1 << 20 // effectively unbounded

t := tlb.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(res).
    WithPorts(ports).
    Build("TLB")
```

If the knob you want doesn't exist yet, adding a `Spec` field is the natural
place to put it (see §3.7).

### 3.3 Observe & Intercept via Hooks

A **hook** is a small object you attach to any `Hookable` (component, port, or
buffer). It fires at named `HookPos` points without modifying the component.
This is the primary tool for **cross-cutting instrumentation**.

```go
import (
    "github.com/sarchlab/akita/v5/hooking"
    "github.com/sarchlab/akita/v5/messaging"
)

// A hook that counts inbound requests on whatever port it's attached to.
type accessCounter struct {
    count *uint64
}

func (h *accessCounter) Func(ctx hooking.HookCtx) {
    if ctx.Pos == messaging.HookPosPortMsgRecvd {
        *h.count++
    }
}

// --- setup ---
var tlbAccesses uint64
topPort.AcceptHook(&accessCounter{count: &tlbAccesses})
```

The same hook can be attached to **every** instance in a loop, so "do X on
every TLB access across the whole machine" is one setup loop, not an edit to
the TLB. Available event points include `messaging.HookPosPortMsgSend` /
`HookPosPortMsgRecvd` on ports and `queueing.HookPosBufPush` /
`HookPosBufPop` on buffers; components may define their own.

Hooks observe and can act on a side-channel you own; for genuine **behavior**
change, prefer §3.1 or §3.5.

### 3.4 Inject Shared State via `Resources` (Dependency Injection)

When several components must share one structure (a page table, a custom
oracle, a metrics object), build it **once** at the top level and inject the
**same reference** into each consumer's `Resources`. This is the legitimate
replacement for a global lookup.

```go
// Build the shared thing once.
pageTable := vm.NewPageTable(12)

// Hand the SAME reference to every consumer at setup.
m := mmu.Definition.Builder().
    WithSimulation(sim).
    WithResources(mmu.Resources{PageTable: pageTable}).
    WithPorts(mmuPorts).
    Build("MMU")
g := gmmu.Definition.Builder().
    WithSimulation(sim).
    WithResources(gmmu.Resources{PageTable: pageTable}).
    WithPorts(gmmuPorts).
    Build("GMMU")
```

At runtime each component uses its held reference directly — no name, no map,
no type assertion, and the dependency is visible in the wiring code. A
component being "global" (one instance shared by many) changes nothing: it's
still build-once, inject-the-same-pointer.

### 3.5 Extend Behavior via Middleware

A component *is* its middleware pipeline: every event it receives — for a
ticking component, a tick every cycle — goes to each field of its
`Middlewares` struct in order. You can splice in a custom stage — to inject a
delay, drop a message, arbitrate differently, or add a probe that needs to run
every cycle — by adding a field to `Middlewares` and creating it in
`NewMiddlewares`. For the TLB:

```go
type Middlewares struct {
    Probe *myProbe // runs first, every cycle
    Ctrl  *ctrlMiddleware
    TLB   *tlbMiddleware
}

type myProbe struct{ comp *Comp }

func (m *myProbe) Handle(_ timing.Event) bool {
    // inspect/act each cycle; return true if progress was made
    return false
}

func newMiddlewares(c *Comp) Middlewares {
    return Middlewares{
        Probe: &myProbe{comp: c},
        Ctrl:  &ctrlMiddleware{comp: c},
        TLB:   &tlbMiddleware{comp: c},
    }
}
```

The middlewares are fixed when `Build` runs — nothing is added to a component
from outside — so this edit lives in the component's package: a variant
(§3.1) or your research fork (§3.7). Like any middleware, the probe holds only
references; anything it counts goes in `State`. Use middleware for per-cycle
behavior you want layered onto an existing component. For a *wholesale*
behavior swap, write a variant (§3.1) instead; to only *observe* from outside,
use a hook (§3.3).

### 3.6 Seed `State` at Setup (Warm-up)

Preloading a cache, pre-filling a TLB, or starting from a crafted condition is
a **setup-time** concern, but the `State` is still written only by the
component's own code. The component gives the system builder an entry point,
and the system builder calls it during assembly, before the simulation runs.
There are two:

- **From configuration.** The `Definition`'s `NewState` builds each instance's
  initial State from its Spec and Resources. A warm-up that a Spec field or a
  Resource can describe belongs here.
- **From a package function.** The component's package exports a function
  that takes the instance and seeds it. `examples/ping` hands a component its
  pings this way:

```go
// In the component's package:
func SchedulePing(
    comp *Comp,
    sendAt timing.VTimeInPicoSec,
    dst messaging.RemotePort,
) {
    state := &comp.State
    state.ScheduledPings = append(state.ScheduledPings, scheduledPing{
        SendAt: sendAt,
        Dst:    dst,
    })
    comp.WakeAt(sendAt)
}

// In the system builder, before Run():
ping.SchedulePing(agentA, 1, agentB.Ports.Out.AsRemote())
```

A function that preloads a TLB's sets would have the same shape; if the
component lacks one, add it in your fork (§3.7). This is *not* the same as
reaching into `State` at **runtime** from another component — that's the
anti-pattern (§5). At setup, you are the assembler; the machine isn't running
yet, so seeding state through the owner's entry point is exactly your job.

### 3.7 Escape Hatch: Fork the Package

If no seam fits — a truly unforeseen experiment — edit the component's package.
Inside `tlb`, every field is visible; add the function, `Spec` flag,
`Middlewares` field, `HookPos`, or behavior you need. This is the right place for deep surgery: the change is
explicit, local to the thing you're studying, and committed to your research
branch — not smuggled in as a runtime poke from across the codebase.

When you find yourself adding a seam others would want (a new `Spec` flag, a
new `HookPos`, an exposed accessor), consider upstreaming it.

---

## 4. Worked Recipes

### 4.1 Perfect TLB / Ideal Memory

**Goal:** every lookup hits, zero latency.
**Seam:** variant component (§3.1), or a `Spec` flag if the component supports
one.
Build the ideal variant in place of the real one. `idealmemcontroller` is the
shipped exemplar for memory; an `idealtlb` would be the equivalent for
translation.

### 4.2 Infinite Cache / Non-Blocking Buffer

**Goal:** never evict, never stall on capacity.
**Seam:** `Spec` config (§3.2). Set capacity to a huge value and/or select a
"no-eviction" policy at build time. A buffer's capacity is a constructor
argument (`queueing.NewBuffer(name, hugeCap)`); a cache's associativity/sets
are `Spec` fields. The thing you wanted to poke is a build-time knob — set it.

### 4.3 Global Counter on Every TLB Access

This is the sharpest case, so it gets the most detail. Start simple, then
handle the variant where the counter lives **inside another component**.

**4.3a — The counter is yours (instrumentation):** attach the §3.3 hook to
each TLB's incoming port; increment a counter you own. Done.

**4.3b — The counter is owned by the global MMU.** Now a TLB access must bump a
counter that is MMU state. Resolve it by the §2 decision guide:

*If it's instrumentation the MMU merely reports* — the cleanest design is to
**not** bury it in MMU state at all. Make it a shared resource (§3.4): one
counter object, injected into both the MMU (which reads it) and the TLBs (or a
hook, which writes it). Nobody reaches into anybody's state.

```go
// Your own counter type, and an Accesses field you add to mmu.Resources.
accesses := &Counter{}
m := mmu.Definition.Builder().
    WithSimulation(sim).
    WithResources(mmu.Resources{PageTable: pageTable, Accesses: accesses}).
    WithPorts(mmuPorts).
    Build("MMU")
// each TLB's port gets a hook that increments accesses
```

*If it must be MMU-internal state* — the MMU's package **declares a mutation
function**, and a hook holding an **injected MMU reference** calls it. The TLB
stays untouched; the MMU's State is still written only by MMU code; only the
one function is exposed. (It is a function, not a method: `mmu.Comp` is an
alias of `ticking.Component[…]`, a type from another package, and Go allows no
methods on it. `ping.SchedulePing` in §3.6 has the same shape.)

```go
// In the mmu package — the MMU chooses its mutation surface:
func RecordTLBAccess(c *Comp) { c.State.TLBAccessCount++ }

// A hook bridges TLB-access events to the MMU, carrying a direct reference
// injected at setup.
type tlbAccessRecorder struct{ mmu *mmu.Comp }
func (h *tlbAccessRecorder) Func(ctx hooking.HookCtx) {
    if ctx.Pos == messaging.HookPosPortMsgRecvd {
        mmu.RecordTLBAccess(h.mmu)
    }
}

// setup:
for _, t := range tlbs {
    t.Ports.Top.AcceptHook(&tlbAccessRecorder{mmu: theMMU})
}
```

*If the count drives MMU behavior or timing* — a direct call is a **bug** (it
mutates MMU state synchronously inside the TLB's tick, outside the event
model). Send a **message**: the TLB notifies the MMU on each access, and the
MMU increments on receipt as a scheduled event. Costlier (traffic per access),
but the only correct option when the count feeds back into what the MMU does.

### 4.4 Fault Injection

**Goal:** drop, corrupt, or delay a specific message; flip a bit at cycle N.
**Seam:** a hook at the port (§3.3) to drop/mutate a message on a side-channel,
a **custom middleware** (§3.5) to inject the fault as a tick-based action, or a
dedicated fault-injector component wired into the topology (§3.1). For
state-level faults (bit flips), expose or add a function in the owner's package
(§3.7) and
drive it from a hook or a scheduled event — never a cross-package field write.

### 4.5 A Custom Structure Shared Across Components

**Goal:** a prefetcher and a cache both consult an oracle you invented.
**Seam:** dependency injection (§3.4). Build the oracle once; inject the same
pointer into both. This is what a global lookup *would* have done, done
correctly: explicit, type-safe, set up once.

### 4.6 Read Another Component's Internal State for a Statistic

**Goal:** record, say, a cache's MSHR occupancy over time.
**Seam, in order of preference:**
1. Record the underlying event with a hook (§3.3) — derive occupancy from
   pushes/pops.
2. Have the owner expose a read accessor (it decides what's observable).
3. Use the reflection-based introspection the monitoring tools already do, for
   read-only inspection.
4. Last resort: add the accessor in the package (§3.7).

---

## 5. Anti-Patterns (and What to Do Instead)

| Anti-pattern | Why it's wrong | Instead |
|---|---|---|
| Global `GetByName(...)` to reach another component at runtime | Service-locator: hides the dependency, needs public fields, string+assertion per access | Inject the reference at setup (§3.4) |
| Writing another package's `State` field directly | Breaks encapsulation: only the component's own code writes its `State` | Call a function the owner's package exposes (§4.3b) |
| Direct cross-component mutation that affects timing | Instantaneous, bypasses the event model → silent timing corruption | Send a message (§4.3, timing case) |
| Public buffer/pipeline fields to allow poking | Freezes the representation, breaks invariants | `Spec` knob, hook, or variant |
| Editing a component in place for a one-off and committing it to `main` | Pollutes the shared model | Variant (§3.1) or a research fork (§3.7) |

---

## 6. Cheat Sheet

```
Want to…                          Reach for…
────────────────────────────────  ──────────────────────────────
Replace a component               Variant component        (§3.1)
Set an extreme knob               Spec config              (§3.2)
Count / log / trace               Hook                     (§3.3)
Share a structure                 Resources injection      (§3.4)
Add per-tick behavior             Middleware               (§3.5)
Preload / warm up                 Seed State at setup      (§3.6)
Mutate another component's state  Owner's package function +
  (instrumentation)                 injected reference     (§4.3b)
Influence another's timing        Message between them     (§2, §4.3)
Anything unforeseen               Fork the package         (§3.7)
```

The throughline: **wire at setup, mutate through declared interfaces, and
observe through hooks.** Every "magic" experiment maps to one of these. The
result is a modification that is explicit, reproducible, and reviewable — which,
for a research instrument, is the whole point.
