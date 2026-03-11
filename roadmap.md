# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware.

## Current State (after M14)

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works with acceptance test
- A-B state implemented in `modeling.Component` (double-buffered: current/next, deep-copy, swap)
- **6 components fully transformed** (Comp eliminated + A-B): idealmemcontroller, TLB, mmuCache, addresstranslator, datamover, simplebankedmemory
- Architecture direction fully clarified and approved by human (issues #145, #150)
- All PRs merged through #36. Code builds and tests pass. CI lint errors from M14 fixed.

## Phase: Architecture Transformation — Component by Component

The strategy is to transform each component following the pattern established in idealmemcontroller (M12):
1. Make middleware read from `GetState()` (A buffer) and write to `GetNextState()` (B buffer)
2. Eliminate Comp wrapper struct (or reduce to thin interface wrapper)
3. Inline all dependency logic into middleware (AddressConverter, AddressToPortMapper, VictimFinder, etc.)
4. Move all runtime data into State as pure serializable structs
5. Remove snapshot/restore conversion layers (State IS the canonical representation)

### Component Difficulty Assessment

| Component | Middlewares | Dependencies | State Complexity | Status |
|-----------|-----------|-------------|-----------------|--------|
| idealmemcontroller | 2 | AddressConverter, Storage | ~10 fields | ✅ DONE (M12) |
| TLB | 2 | AddressToPortMapper | Sets, MSHR, Pipeline, Buffer | ✅ DONE (M13) |
| mmuCache | 2 (ctrl+data) | None significant | Sets, FlushReq | ✅ DONE (M14) |
| addresstranslator | 1 | 2× AddressToPortMapper | Transactions | ✅ DONE (M14) |
| datamover | 1 | 2× AddressToPortMapper | Inflight reqs | ✅ DONE (M14) |
| simplebankedmemory | 1 | AddressConverter, Storage | Banks with pipelines | ✅ DONE (M14) |
| GMMU (mmu) | 1 | PageTable (external) | Translations, migration | 🔜 M15 |
| switch | 1 | RoutingTable, Arbiter | Port mappings, buffers, pipelines | 🔜 M15 |
| endpoint | 1 | None significant | Msg assembly/disassembly | 🔜 M15 |
| writearound cache | 1 (monolithic) | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory, MSHR | M16 |
| writeevict cache | 1 (monolithic) | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory, MSHR | M16 |
| writethrough cache | 1 (monolithic) | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory, MSHR | M16 |
| writeback cache | 1 (monolithic, 6 stages) | Directory, MSHR, Storage, AddrToPortMapper, VictimFinder | Huge state (60+ fields) | M17 |
| DRAM | 1 | AddrConverter, SubTransSplitter, AddrMapper, CmdQueue, Channel, Banks | Banks, channels, queues | M18 |
| tickingping (example) | 1 | None | Simple | Trivial (skip) |
| ping (example) | N/A | N/A | Not modeling.Component | N/A |

### M15: GMMU + Switch + Endpoint — NEXT (Issue #181)
- GMMU: eliminate topPort/bottomPort from Comp, keep pageTable as external ref in middleware, use A-B state
- Switch: move ports/portComplex/routingTable/arbiter to middleware, keep runtime pipelines/buffers, reduce Comp
- Endpoint: move all runtime fields to State, eliminate snapshot/restore conversion, use A-B state
- **Budget**: 5 cycles

### M16: Cache Architecture — MSHR/Directory Decoupling + Write{around,evict,through}
- These 3 caches share similar structure (Directory, MSHR, Storage, single middleware)
- MSHR data → State, behavior → free functions on State
- Directory data → State, behavior → free functions on State
- Inline AddressToPortMapper, AddressConverter, VictimFinder
- **Budget**: 8 cycles

### M17: Writeback Cache — Full Transformation
- Most complex component: 60+ State fields, 6 internal stages, complex pipeline
- MSHR/Directory decoupling (reuse patterns from M16)
- Eliminate Comp wrapper
- Remove ~500-line snapshot/restore conversion layer
- **Budget**: 10 cycles

### M18: DRAM Memory Controller
- Inline SubTransSplitter, CommandQueue, Channel, Banks, AddressMapper
- Bank state, channel state, queue contents → State
- Timing tables → Spec
- **Budget**: 8 cycles

### M19: Multi-Middleware Split (stretch goal)
- Split monolithic single-middleware components into multiple middlewares
- This is the "historical issue" the human mentioned — each component should have multiple middlewares
- Affects caches, DRAM, mmu, switch, etc.
- **Budget**: 12 cycles

## ✅ Completed Milestones

| Milestone | Budget | Used | Description |
|-----------|--------|------|-------------|
| M1 | 6 | 5 | Create `modeling` package |
| M2 | 6 | 4 | Refactor idealmemcontroller |
| M3 | 8 | 6 | Save/Load with acceptance test |
| M4 | 3 | 2 | Fix CI lint failures |
| M5 | 8 | 6 | Messages as plain structs |
| M6 | 16 | 8 | Port all first-party components |
| M7 | 30 | 16 | Move mutable data into State |
| M8 | 24 | 18 | Msg-as-Interface redesign |
| M9 | 4 | 2 | Component creation guide |
| M10 | 2 | 3 | CI fix + Dependabot |
| M11 | 2 | 0 | Architecture design finalized |
| M12 | 5 | 3 | A-B state + Comp elimination on idealmemcontroller |
| M13 | 5 | 3 | TLB — Comp elimination + A-B state |
| M14 | 6 | 3 | Simple Components Batch (mmuCache, addresstranslator, datamover, simplebankedmemory) |

## Summary Statistics
- Total milestones completed: 14
- PRs merged: 36
- Components ported: 16/16
- Components fully transformed (Comp eliminated + A-B): 6/16

## Lessons Learned
- CI can get stuck in "queued" state — don't waste cycles waiting for it
- Architecture discussions should be fully resolved before implementation
- Multi-worker mechanical changes work well with clear patterns
- Breaking milestones to 2-6 cycle budgets is optimal
- Human feedback drives direction — stay responsive
- Combined milestones work when scope is small — M14 (4 components) done in 3 cycles
- idealmemcontroller is the reference implementation — follow its patterns
- The snapshot/restore conversion layer disappears when State is canonical (big code reduction)
- A-B state deep copy via JSON round-trip is acceptable for small States
- Lint errors from multi-branch merges should be caught BEFORE merging to main (run linter locally)
- Components with external services (Storage, PageTable, RoutingTable) keep those as middleware fields — they are external substrate, not internal state
