# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework toward a clean, minimal component model inspired by digital circuit semantics.

### Core Component Model

A component is exactly 5 things: **Spec, State, Ports, Middleware, Hooks**. Nothing else.

- **Spec**: Immutable configuration. Primitive/JSON-friendly values. Set once by builder. Includes algorithm parameters (e.g., interleaving sizes, capacities, timing tables) that previously lived in dependency objects.
- **State**: ALL mutable data. Plain serializable structs only (no pointers, interfaces, channels). This is the single source of truth during tick. Data previously held by runtime objects (MSHR entries, directory sets, pipeline stages, bank states) must be represented as pure data in State.
- **Ports**: Communication channels. Accessed via `GetPortByName()`. Port names stored as strings in Spec for routing.
- **Middleware**: Tick logic. Reads current State + Spec, writes next State, sends/receives through Ports, may invoke Hooks. Each middleware is independent — no shared runtime objects between middlewares. May hold `*mem.Storage` as the sole external reference (Storage is physical substrate, not internal state).
- **Hooks**: Extension points for monitoring/instrumentation.

### A-B State (Double-Buffered) — IMPLEMENTED in modeling.Component

Each component has TWO state copies: "current" (read-only during tick) and "next" (write-only during tick). Before middleware runs, `current` is deep-copied to `next`. After all middleware finishes, `next` becomes `current`. This matches digital circuit semantics where registers read old values and write new values in the same clock cycle.

- `GetState()` → returns current (A buffer, read-only)
- `GetNextState()` → returns pointer to next (B buffer, write-only)
- `SetNextState()` → sets next buffer directly
- `SetState()` → sets both buffers (for initialization/save-load)
- Serialization saves only `current` state

Human clarifications:
- Single-middleware patterns (e.g., writeback cache with one middleware running all stages) are historical — components SHOULD have multiple middlewares.
- Deferring visibility to next cycle is acceptable even if it slightly changes behavior.
- Middleware should ONLY work with State, read from Spec, send/receive through Ports, and invoke Hooks.

### No Dependencies / No Comp Wrapper

- **Eliminate ALL Comp wrapper structs.** Use `modeling.Component[Spec, State]` directly. (Exception: thin wrappers for StorageOwner interface are acceptable.)
- **Eliminate external dependencies** (e.g., AddressToPortMapper, VictimFinder, AddressConverter, SubTransSplitter, CommandQueue interfaces). Inline the logic directly into middleware instead. "A little duplication is better than a little dependency." (Rob Pike)
- Dependencies create problems with A-B state (e.g., port routing must happen immediately, breaking next-cycle-visibility). Embedding logic in middleware avoids this.
- Store port *names* (strings) in Spec, resolve via `GetPortByName()` at runtime. Port routing reads Spec (immutable), `Send()` is a side-effect on the network — not internal state.
- `*mem.Storage` is the ONE allowed external reference per middleware (physical memory substrate, cannot be State due to size, sharing, mutexes).
- Similarly, `vm.PageTable` (MMU) and `routing.Table` / `arbitration.Arbiter` (Switch) are external services — acceptable as middleware fields, like Storage.

### MSHR and Directory as State + Free Functions (DONE)

Runtime objects like MSHR and Directory contain both data and behavior. Following the principle that **State holds data, middleware holds behavior**:

- **MSHR**: `capacity` → Spec. `entries []MSHREntry` → State as `MSHRState`. Behavior (`Query`, `Add`, `Remove`, `IsFull`) → free functions operating on `*MSHRState` + Spec values.
- **Directory**: `sets []Set` → State as `DirectoryState`. Behavior (`Lookup`, `FindVictim`, `Visit`) → free functions operating on `*DirectoryState`. LRU victim finding uses the LRU queue already stored in `DirectoryState`.
- These free functions live in `mem/cache/` and are shared by all cache types (writearound, writeevict, writethrough, writeback).
- Block/entry cross-references use **indices** (setID, wayID, transaction index) instead of pointers.

### Messages as Concrete Types (DONE)

`sim.Msg` is an interface with `Meta() *MsgMeta`. Each package defines concrete, serializable message types embedding `sim.MsgMeta`. No builders, no msgRef types. Components type-switch on concrete types.

### Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods. Components implement `StateSaver`/`StateLoader`. After Comp elimination, the snapshot/restore conversion layers disappear — State IS the canonical representation.

### Multi-Middleware Split (NEXT PHASE)

All components currently use a single monolithic middleware. The human has explicitly stated that this is a historical artifact and components SHOULD have multiple middlewares. This is the next major phase:

- Each component should be decomposed into multiple middleware stages, each responsible for one logical function (e.g., parse incoming, process pipeline stage, respond).
- Under A-B state semantics, each middleware reads from `current` (A buffer) and writes to `next` (B buffer). Middlewares within the same tick do NOT see each other's writes — this matches hardware pipeline register semantics.
- The 1-cycle latency per middleware boundary is acceptable (per human clarification).
- This change needs careful component-by-component analysis to identify natural stage boundaries.

### Runtime Objects to State (REMAINING GAPS)

Some components still have runtime objects that should be canonical State:
- **Switch**: Pipeline stages, route/forward/sendOut buffers are runtime `queueing.Pipeline`/`queueing.Buffer` objects. These should become pure data in State with behavior as free functions.
- **Endpoint**: Similar buffer runtime objects.
- **MMU**: Runtime transactions (`[]transaction` with live message pointers), page access tracking, migration state on Comp — these need to become canonical State with SaveState/LoadState conversion eliminated.

## How You Consider the Project is Success

- Simple, straightforward, intuitive APIs
- All CI checks pass on main branch
- Component = Spec + State + Ports + Middleware + Hooks (nothing else)
- No Comp wrapper structs (except thin wrappers for StorageOwner / external service interfaces)
- No external dependency interfaces — logic embedded in middleware
- A-B state pattern correctly used in all components (GetState for read, GetNextState for write)
- Data from runtime objects (MSHR, directory, pipeline, buffers) lives in State as pure data
- No SaveState/LoadState conversion layers — State IS canonical
- Acceptance test for save/load process passes
- All first-party components use the modeling package pattern
- Each component has multiple middlewares (not one monolithic middleware)
- component_guide.md reflects the final architecture

## Constraints

- Keep State pure and serializable (no pointers, live handles, functions, channels)
- Keep Spec primitive and JSON-friendly
- Use tick-driven patterns; prefer countdowns over scheduled events
- Middleware reads current State (read-only) and writes next State (write-only)
- A little duplication is better than a little dependency
- `*mem.Storage` is the sole allowed external reference held by middleware (PageTable, RoutingTable, Arbiter are acceptable external service references)
- Deep copy uses JSON round-trip (validated by ValidateState — no pointers)
- 1-cycle delay from A-B buffering is acceptable for multi-middleware components

## Resources

- Diana's A-B state co-design analysis: `workspace/diana/ab_state_comp_elim_codesign.md`
- Iris's dependency elimination analysis: `workspace/iris/embed_logic_in_middleware_analysis.md`
- Iris's MSHR decoupling analysis: `workspace/iris/mshr_dependency_analysis.md`
- Human approvals: Issues #145 (Comp elimination), #150 (A-B state)
- Idealmemcontroller serves as the reference implementation for thin Comp + 2 middlewares
- Writeback cache (mem/cache/writeback) is the reference for no-Comp with State as canonical
- DRAM (mem/dram) is the reference for inlined dependencies with free functions
