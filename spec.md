# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework toward a clean, minimal component model.

### Core Component Model

A component is exactly 5 things: **Spec, State, Ports, Middleware, Hooks**. Nothing else.

- **Spec**: Immutable configuration. Primitive/JSON-friendly. Set once by builder.
- **State**: ALL mutable data. Plain serializable structs only (no pointers, interfaces, channels). This is the single source of truth during tick.
- **Ports**: Communication channels. Accessed via `GetPortByName()`.
- **Middleware**: Tick logic. Reads current State + Spec, writes next State, sends/receives through Ports, may invoke Hooks. Each middleware is independent — no shared runtime objects.
- **Hooks**: Extension points for monitoring/instrumentation.

### A-B State (Double-Buffered)

Each component has TWO state copies: "current" (read-only during tick) and "next" (write-only during tick). After all middleware finishes, swap current↔next. This matches digital circuit semantics. Serialization only saves current state.

Human clarifications:
- Single-middleware patterns (e.g., writeback cache with one middleware running all stages) are historical — components SHOULD have multiple middlewares.
- Deferring visibility to next cycle is acceptable even if it slightly changes behavior.
- Middleware should ONLY work with State, read from Spec, send/receive through Ports, and invoke Hooks.

### No Dependencies / No Comp Wrapper

- **Eliminate ALL Comp wrapper structs.** Use `modeling.Component[Spec, State]` directly.
- **Eliminate external dependencies** (e.g., AddressToPortMapper, VictimFinder, AddressConverter interfaces). Duplicate the logic directly into middleware instead. "A little duplication is better than a little dependency." (Rob Pike)
- Dependencies create problems with A-B state (e.g., port routing must happen immediately, breaking next-cycle-visibility). Embedding logic in middleware avoids this.

### Messages as Concrete Types (DONE)

`sim.Msg` is an interface with `Meta() *MsgMeta`. Each package defines concrete, serializable message types embedding `sim.MsgMeta`. No builders, no msgRef types. Components type-switch on concrete types.

### Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods. Components implement `StateSaver`/`StateLoader`.

## Success Criteria

- Simple, straightforward, intuitive APIs
- All CI checks pass on main branch
- Component = Spec + State + Ports + Middleware + Hooks (nothing else)
- No Comp wrapper structs
- No external dependency interfaces — logic embedded in middleware
- A-B state pattern implemented
- Acceptance test for save/load process passes
- All first-party components use the modeling package pattern

## Constraints

- Keep State pure and serializable (no pointers, live handles, functions, channels)
- Keep Spec primitive and JSON-friendly
- Use tick-driven patterns; prefer countdowns over scheduled events
- Middleware reads current State (read-only) and writes next State (write-only)
- A little duplication is better than a little dependency
