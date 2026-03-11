# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework toward a clean, minimal component model inspired by digital circuit semantics.

### Core Component Model

A component is exactly 5 things: **Spec, State, Ports, Middleware, Hooks**. Nothing else.

- **Spec**: Immutable configuration. Primitive/JSON-friendly values. Set once by builder. Includes algorithm parameters (e.g., interleaving sizes, capacities, timing tables, port names for routing) that previously lived in dependency objects.
- **State**: ALL mutable data. Plain serializable structs only (no pointers, interfaces, channels). This is the single source of truth during tick. Data previously held by runtime objects (MSHR entries, directory sets, pipeline stages, bank states, buffers) must be represented as pure data in State. Behavior from former runtime objects (MSHR.Query, Directory.Lookup, Buffer.Push, Pipeline.Tick) becomes **free functions** operating on `*State` and `*Spec`.
- **Ports**: Communication channels. Accessed via `GetPortByName()`. Port names stored as strings in Spec for routing.
- **Middleware**: Tick logic. Each middleware is a self-contained stage. Reads current State + Spec, writes next State, sends/receives through Ports, may invoke Hooks. Each middleware is independent — no shared mutable objects between middlewares. May hold `*mem.Storage` (or similar physical substrate references) as the sole external reference.
- **Hooks**: Extension points for monitoring/instrumentation.

### A-B State (Double-Buffered) — IMPLEMENTED in modeling.Component

Each component has TWO state copies: "current" (read-only during tick) and "next" (write-only during tick). Before middleware runs, `current` is deep-copied to `next`. After all middleware finishes, `next` becomes `current`. This matches digital circuit semantics where registers read old values and write new values in the same clock cycle.

- `GetState()` → returns current (A buffer, read-only)
- `GetNextState()` → returns pointer to next (B buffer, write-only)
- `SetNextState()` → sets next buffer directly
- `SetState()` → sets both buffers (for initialization/save-load)
- Serialization saves only `current` state

### Multi-Middleware Architecture

All components should have **multiple middlewares**, each responsible for one logical function. This is the target architecture — single-middleware patterns are historical artifacts.

- Under A-B state semantics, each middleware reads from `current` (A buffer) and writes to `next` (B buffer). Middlewares within the same tick do NOT see each other's writes — this matches hardware pipeline register semantics.
- The +1 cycle latency per middleware boundary is acceptable (per human clarification).
- Components should be decomposed into natural stage boundaries (e.g., parse → process → respond).

### No Dependencies — Inline All Logic

**"A little duplication is better than a little dependency."** (Rob Pike)

- **Eliminate ALL external dependency interfaces** (AddressToPortMapper, VictimFinder, AddressConverter, SubTransSplitter, CommandQueue, etc.). Inline the logic directly into middleware.
- Dependencies create problems with A-B state (e.g., port routing must happen immediately, breaking next-cycle-visibility). Embedding logic in middleware avoids this entirely.
- Store port *names* (strings) in Spec, resolve via `GetPortByName()` at runtime. Port routing reads Spec (immutable), `Send()` is a side-effect on the network — not internal state.

### Allowed External References

The following external references may be held as middleware fields — they are **physical substrate or shared services**, not internal state:

| Reference | Components | Rationale |
|-----------|-----------|-----------|
| `*mem.Storage` | idealmemcontroller, caches, DRAM | Physical memory substrate. Too large to copy, can be shared. |
| `vm.PageTable` | MMU | Shared OS-level page table. Not component-internal state. |
| `routing.Table` | Switch | Network routing table. Shared infrastructure. |
| `arbitration.Arbiter` | Switch | Arbitration policy. External service. |

No other external references should exist in middleware. Everything else is either Spec (immutable config) or State (mutable data).

### No Comp Wrapper Structs

- **Eliminate ALL Comp wrapper structs.** Use `modeling.Component[Spec, State]` directly.
- Exception: thin wrappers for `StorageOwner` interface are acceptable until a better pattern emerges.

### MSHR and Directory as State + Free Functions (DONE)

Runtime objects like MSHR and Directory contain both data and behavior. Following the principle that **State holds data, middleware holds behavior**:

- **MSHR**: `capacity` → Spec. `entries []MSHREntry` → State as `MSHRState`. Behavior → free functions on `*MSHRState`.
- **Directory**: `sets []Set` → State as `DirectoryState`. Behavior → free functions on `*DirectoryState`. LRU victim finding uses the LRU queue already stored in State.
- Block/entry cross-references use **indices** (setID, wayID, transaction index) instead of pointers.
- Shared free functions in `mem/cache/` reusable across all cache types.

### Pipeline and Buffer as State + Free Functions (NEXT)

Following the same data/behavior separation as MSHR and Directory:

- **Pipeline stages** → State as arrays/slices. Pipeline tick/accept → free functions on State.
- **Buffer contents** → State as arrays/slices. Buffer push/pop → free functions on State.
- This eliminates `queueing.Pipeline` and `queueing.Buffer` runtime objects from middleware.
- The `restoreFromState` conversion layers disappear — State IS the canonical representation.

### Messages as Concrete Types (DONE)

`sim.Msg` is an interface with `Meta() *MsgMeta`. Each package defines concrete, serializable message types embedding `sim.MsgMeta`. No builders, no msgRef types. Components type-switch on concrete types.

### Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods. After Comp elimination and State-as-canonical, snapshot/restore conversion layers disappear.

## How You Consider the Project is Success

- Simple, straightforward, intuitive APIs
- All CI checks pass on main branch
- Component = Spec + State + Ports + Middleware + Hooks (nothing else)
- No Comp wrapper structs (except thin wrappers for StorageOwner / external service interfaces)
- No external dependency interfaces — all logic embedded in middleware
- A-B state pattern correctly used in all components (GetState for read, GetNextState for write)
- Data from all runtime objects (MSHR, directory, pipeline, buffers) lives in State as pure data
- No SaveState/LoadState conversion layers — State IS canonical
- No restoreFromState / syncToState functions — middleware works directly with State
- No runtime copies of State substructures in middleware
- Acceptance test for save/load process passes
- All first-party components use the modeling package pattern
- Each component has multiple middlewares (not one monolithic middleware)
- `component_guide.md` reflects the final architecture

## Constraints

- Keep State pure and serializable (no pointers, live handles, functions, channels)
- Keep Spec primitive and JSON-friendly
- Use tick-driven patterns; prefer countdowns over scheduled events
- Middleware reads current State (read-only) and writes next State (write-only)
- A little duplication is better than a little dependency
- `*mem.Storage`, `vm.PageTable`, `routing.Table`, `arbitration.Arbiter` are the only allowed external references held by middleware
- Deep copy uses JSON round-trip (validated by ValidateState — no pointers)
- 1-cycle delay from A-B buffering is acceptable for multi-middleware components

## Resources

- Diana's A-B state co-design analysis: `workspace/diana/ab_state_comp_elim_codesign.md`
- Iris's dependency elimination analysis: `workspace/iris/embed_logic_in_middleware_analysis.md`
- Iris's MSHR decoupling analysis: `workspace/iris/mshr_dependency_analysis.md`
- Iris's embedded logic analysis: `workspace/iris/embed_logic_in_middleware_analysis.md`
- Human approvals: Issues #145 (Comp elimination), #150 (A-B state)
- Reference implementations:
  - `mem/idealmemcontroller/` — 2 middleware, thin Comp, A-B state
  - `mem/cache/writeback/` — State canonical for MSHR/Directory, free functions
  - `mem/dram/` — inlined dependencies, free functions
  - `noc/networking/switching/switches/` — State canonical, pipeline/buffer in State
