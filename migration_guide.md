# Migration Guide

## Defining Components in V5: Philosophy and Patterns

V5 unifies how components are modeled and wired. Each component is a single struct composed of four orthogonal parts: Spec, State, Ports, and Middlewares. The goals are: declarative configuration, local and serializable runtime state, explicit wiring, testability, and deterministic snapshot/restore.

### Core Principles

1. Spec (immutable configuration)
   - Describes behavior and dependencies using only primitives (bool, number, string) and primitive maps/slices.
   - Strategy dependencies are expressed as small primitive "sub‑specs" (e.g., `{ Kind: "interleaving", Params: { ... } }`).
   - No pointers or live objects in Spec. Keep it JSON/YAML‑friendly and hashable.
   - Validation and defaults are part of the component package (e.g., `validate()` + `defaults()`).

2. State (mutable runtime data)
   - Pure data only: scalars and slices/maps of primitives or simple structs thereof.
   - No live handles, functions, channels, or ports in State.
   - All cross‑references use stable identifiers (IDs), never in‑memory pointers.
   - Snapshot/restore uses deep copies of State so checkpoints are immutable.

3. Ports (externally injected)
   - Components never construct or own connections. Ports are created/injected during wiring and registered via `AddPort(name, port)`.
   - Components access ports by name via `GetPortByName("...")` to avoid compile‑time coupling.

4. Middlewares (ordered, stateless over the component)
   - Implement the per‑tick pipeline. Each middleware operates on the component’s State and interacts with Ports.
   - Keep middlewares stateless wrt external dependencies; resolve them at build time and pass the resolved handles in.
   - Prefer tick‑driven countdowns/backpressure over ad‑hoc scheduled events for simpler snapshots and determinism.

### Dependency Injection and Shared State

- Strategy injection (e.g., address conversion)
  - Keep in Spec as a primitive descriptor (`Kind`, `Params`), not as a live object.
  - Resolve to concrete implementations locally in the component builder and inject into middlewares.
  - On restore, reconstruct from Spec; never serialize strategy objects.

- Emulation state (e.g., memory storage)
  - Treat as shared state separate from timing logic. Store only an ID (e.g., `StorageRef`) in Spec/State.
  - Keep a per‑simulation state registry; components resolve handles by ID at runtime.
  - Snapshot/restore orchestrates shared state once per ID (outside components); components snapshot only their own State.

### Build and Wire (two stages)

1. Build from Spec
   - `Builder.WithSpec(spec).WithSimulation(sim).Build(name)` constructs the component with defaults and resolved strategies.
   - Do not create or connect ports here.

2. Wire topology
   - Create ports and connections, then inject ports via `AddPort("...", port)`.
   - Use names consistently so components and tooling can introspect topology.

### Determinism and Introspection

- Determinism: avoid non‑deterministic IDs or iteration order; snapshot ID generators; canonicalize map iteration by sorting.
- Introspection: provide methods to inspect effective Spec (with defaults) and to dump State for debugging.
- Tracing/metrics: attach as middlewares or hooks; avoid embedding tracing in business logic.

### Testing and Mocks

- Favor local interfaces inside the component package to reduce external coupling (e.g., `Storage`, `AddressConverter`, `StateAccessor`).
- Generate mocks from local interfaces for unit tests; avoid importing remote mocks.
- Drive behavior via ticks and ports; avoid requiring real engines or networks in unit tests.

### Example: Ideal Memory Controller (V5)

- Spec
  - Timing: `Width`, `LatencyCycles`, `Freq`.
  - Shared emulation: `StorageRef` (ID in simulation state registry).
  - Strategy: `AddrConv` as `{ Kind, Params }` (e.g., identity/interleaving).

- State
  - Pure data transactions with countdowns; no ports or live pointers.
  - Drain/enable mode as a small enum; deep‑copied for snapshots.

- Ports
  - `Top`, `Control` injected during wiring; accessed via name lookups.

- Middlewares
  - Data path: tick‑driven; consumes from `Top`, counts down latency, responds when ready; uses storage resolved via state registry by `StorageRef`.
  - Control path: processes enable/pause/drain; replies only when safe (e.g., after drain completes).

This pattern generalizes to other components: keep Spec primitive and declarative, keep State pure and serializable, inject Ports, and implement behavior as pipelines of middlewares with minimal, explicit dependencies.

## CLI Changes (akitav5)

- Command rename: `akita check [path]` is replaced by `akita component-lint [path]`.
  - Usage examples:
    - `akita component-lint .`
    - `akita component-lint ./...`
    - `akita component-lint -r mem/`
  - Note: directories without `//akita:component` are reported as `not a component` and do not fail the run.
- New scaffolding entry point: use `akita component-create <path>` instead of the previous `component --create` flag.
  - Example: `akita component-create mem/newcontroller`
  - The command requires running inside the Akita Git repository so that generated packages start with a valid module path.
