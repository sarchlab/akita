# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework toward a clean, minimal component model inspired by digital circuit semantics.

### Ultimate Goal (from human, issue #342)

1. **Single simulation-level save and load support** — one call saves/loads the entire simulation state.
2. **No per-component custom save/load functions** — serialization must be fully automatic.
3. **Developers focus only on component logic** — ideally, only middleware Tick functions need to be implemented. No boilerplate.
4. **No compromise in performance** — must match original akita repo performance.

### Core Component Model

A component is exactly 5 things: **Spec, State, Ports, Middleware, Hooks**. Nothing else.

- **Spec**: Immutable configuration. Primitive/JSON-friendly values. Set once by builder. Includes algorithm parameters (e.g., interleaving sizes, capacities, timing tables, port names for routing) that previously lived in dependency objects.
- **State**: ALL mutable data. Must be serializable. Can include:
  - Plain structs with primitive fields
  - Buffers (implementing a Serialize interface)
  - Pipelines (implementing a Serialize interface)
  - Any object that implements a serialization interface
  
  Data previously held by runtime objects (MSHR entries, directory sets, pipeline stages, bank states, buffers) must be represented as pure data in State. Behavior from former runtime objects (MSHR.Query, Directory.Lookup, Buffer.Push, Pipeline.Tick) becomes **free functions** operating on `*State` and `*Spec`.
- **Ports**: Communication channels. Accessed via `GetPortByName()`. Port names stored as strings in Spec for routing.
- **Middleware**: Tick logic. Each middleware is a self-contained stage. Reads/writes State, sends/receives through Ports, may invoke Hooks. Each middleware is independent — no shared mutable objects between middlewares. May hold `*mem.Storage` (or similar physical substrate references) as the sole external reference.
- **Hooks**: Extension points for monitoring/instrumentation.

### In-Place State Update — IMPLEMENTED in modeling.Component (M34)

Component uses in-place state update: `current` and `next` refer to the same state value. During Tick, `current` is assigned to `next` before the middleware pipeline runs; after the pipeline completes, `next` is assigned back to `current`. Because both point to the same value, middlewares can read from GetState or GetNextState interchangeably.

- `GetState()` → returns current state
- `GetNextState()` → returns pointer to next state (same underlying data)
- `SetNextState()` → sets next state directly
- `SetState()` → sets both current and next (for initialization/save-load)
- Serialization saves only `current` state
- No deep copy overhead (0µs per tick)

### Serializable State (issue #343)

State can contain buffers, pipelines, and any other object that implements a serialization interface. This allows:
- Buffers as first-class state members (not adapter wrappers)
- Pipelines as first-class state members
- Any serializable type as a state member
- Automatic save/load without per-component custom code

**Discussion needed**: How to handle pipelines in state. What serialization interface to use.

### Multi-Middleware Architecture — DONE

All components have **multiple middlewares**, each responsible for one logical function. Single-middleware patterns are eliminated.

- With in-place state update, middlewares within the same tick CAN see each other's writes. This is by design — it matches the read-own-writes pattern used by all components.
- The +1 cycle latency per middleware boundary is acceptable (per human clarification).
- Components are decomposed into natural stage boundaries (e.g., pipeline + control, parse + respond).

**Current status**: All 16/16 components have multiple middlewares (2-3 each).

### No Dependencies — Inline All Logic

**"A little duplication is better than a little dependency."** (Rob Pike)

- **Eliminate ALL external dependency interfaces** (AddressToPortMapper, VictimFinder, AddressConverter, SubTransSplitter, CommandQueue, etc.). Inline the logic directly into middleware.
- Store port *names* (strings) in Spec, resolve via `GetPortByName()` at runtime. Port routing reads Spec (immutable), `Send()` is a side-effect on the network — not internal state.

### Allowed External References

The following external references may be held as middleware fields — they are **physical substrate or shared services**, not internal state:

| Reference | Components | Rationale |
|-----------|-----------|-----------|
| `*mem.Storage` | idealmemcontroller, caches, DRAM, simplebankedmemory | Physical memory substrate. Too large to copy, can be shared. |
| `vm.PageTable` | MMU, GMMU | Shared OS-level page table. Not component-internal state. |
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

### Pipeline and Buffer as State + Free Functions (DONE in caches + switch)

Following the same data/behavior separation as MSHR and Directory:

- **Pipeline stages** → State as arrays/slices. Pipeline tick/accept → free functions on State.
- **Buffer contents** → State as arrays/slices. Buffer push/pop → free functions on State.
- This eliminates `queueing.Pipeline` and `queueing.Buffer` runtime objects from middleware.
- The `restoreFromState` conversion layers disappear — State IS the canonical representation.

**Future direction** (issue #343): Buffers and pipelines should implement a serialization interface so they can be embedded directly in State as first-class objects, without needing adapter wrappers or manual state conversion.

### Messages as Concrete Types (DONE)

`sim.Msg` is an interface with `Meta() *MsgMeta`. Each package defines concrete, serializable message types embedding `sim.MsgMeta`. No builders, no msgRef types. Components type-switch on concrete types.

### Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods. After Comp elimination and State-as-canonical, snapshot/restore conversion layers disappear.

## Open Issues (from human review)

### Resolved
1. ~~**Unnecessary Comp wrapper structs**~~: Fixed in M29.
2. ~~**Middleware boilerplate**~~: Fixed in M29.
3. ~~**File naming**~~: Fixed in M29.
4. ~~**Simulation performance regression**~~: Fixed in M34 — eliminated deep copy entirely (in-place state update, 0µs overhead).
5. ~~**NOC test size revert**~~: Fixed in M34 — reverted to original upstream values.

**Human decision on sim package**: Keep sim package as-is. Do NOT split.

### Active

6. **Cache unification** (issues #321, #336): Merge 3 simpler caches (writearound, writeevict, writethrough) into one cache with WritePolicy strategy. **Human approved for implementation** (issue #336 comment: "Let's go with merging the 3 simpler caches"). Writeback stays separate.

7. **Buffers and pipelines in state** (issue #343): Allow buffers and anything serializable as part of state by implementing a serialize interface. Discuss how to handle pipelines. This connects to the ultimate goal of automatic save/load without per-component custom code.

8. **Global state manager** (issue #326): Long-term direction. Register all state centrally with string-based identifiers. Deferred due to performance concerns (map access 75× slower than struct fields). May revisit as optional overlay for tooling/debugging.

### CI Infrastructure

All CI workflow jobs use `self-hosted` runners per human directive (issue #309). GitHub-hosted runners are not an option due to budget constraints. All 5 CI jobs have `timeout-minutes` set to prevent hanging (issue #310). Self-hosted runners may be temporarily unavailable (queued) — wait for them to come online.

## How You Consider the Project is Success

1. Simple, straightforward, intuitive APIs
2. All CI checks pass on main branch
3. Component = Spec + State + Ports + Middleware + Hooks (nothing else)
4. No Comp wrapper structs (except thin wrappers for StorageOwner / external service interfaces)
5. No external dependency interfaces — all logic embedded in middleware
6. Single simulation-level save/load (no per-component custom save/load)
7. Developers only need to implement middleware Tick functions
8. Data from all runtime objects (MSHR, directory, pipeline, buffers) lives in State as pure data
9. No SaveState/LoadState conversion layers — State IS canonical
10. No restoreFromState / syncToState functions — middleware works directly with State
11. No runtime copies of State substructures in middleware
12. Acceptance test for save/load process passes
13. All first-party components use the modeling package pattern
14. Each component has multiple middlewares (not one monolithic middleware)
15. `component_guide.md` reflects the final architecture
16. Performance matches original akita repo (no compromise)

## Constraints

- State must be serializable (can include types implementing serialize interface)
- Keep Spec primitive and JSON-friendly
- Use tick-driven patterns; prefer countdowns over scheduled events
- In-place state update: middlewares read/write the same state within a tick
- A little duplication is better than a little dependency
- `*mem.Storage`, `vm.PageTable`, `routing.Table`, `arbitration.Arbiter` are the only allowed external references held by middleware
- No per-component custom save/load functions
- No compromise in performance

## Resources

- Diana's A-B state co-design analysis: `workspace/diana/ab_state_comp_elim_codesign.md`
- Iris's dependency elimination analysis: `workspace/iris/embed_logic_in_middleware_analysis.md`
- Iris's MSHR decoupling analysis: `workspace/iris/mshr_dependency_analysis.md`
- Iris's cache unification design: `workspace/iris/note.md` (issue #336)
- Diana's in-place update analysis: `workspace/diana/note.md` (issue #335)
- Human approvals: Issues #145 (Comp elimination), #150 (A-B state), #336 (cache unification)
- Reference implementations:
  - `mem/idealmemcontroller/` — 2 MW, thin Comp (StorageOwner)
  - `mem/cache/writeback/` — State canonical for MSHR/Directory, free functions
  - `mem/cache/writearound/` — legacyMapper resolved at Build time
  - `mem/dram/` — 3 MW, inlined dependencies, free functions
  - `noc/networking/switching/switches/` — 2 MW, State canonical, pipeline/buffer in State
  - `noc/networking/switching/endpoint/` — 2 MW
  - `mem/simplebankedmemory/` — 2 MW
  - `examples/tickingping/` — 2 MW
