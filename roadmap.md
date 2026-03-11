# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware.

## Current State (after M12)

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works with acceptance test
- A-B state implemented in `modeling.Component` (double-buffered: current/next, deep-copy, swap)
- idealmemcontroller fully transformed: Comp reduced to thin StorageOwner, AddressConverter inlined, middleware reads A/writes B
- Architecture direction fully clarified and approved by human (issues #145, #150)
- All PRs merged through #32. Code builds and tests pass.

## Phase: Architecture Transformation — Component by Component

The strategy is to transform each component following the pattern established in idealmemcontroller (M12):
1. Make middleware read from `GetState()` (A buffer) and write to `GetNextState()` (B buffer)
2. Eliminate Comp wrapper struct (or reduce to thin StorageOwner)
3. Inline all dependency logic into middleware (AddressConverter, AddressToPortMapper, VictimFinder, etc.)
4. Move all runtime data into State as pure serializable structs
5. Remove snapshot/restore conversion layers (State IS the canonical representation)

### Component Difficulty Assessment

| Component | Middlewares | Dependencies | State Complexity | Difficulty |
|-----------|-----------|-------------|-----------------|-----------|
| idealmemcontroller | 2 | AddressConverter, Storage | ~10 fields | ✅ DONE (M12) |
| TLB | 2 | AddressToPortMapper | Sets, MSHR, Pipeline, Buffer | Medium |
| mmuCache | 2 (ctrl+data) | None significant | Sets, FlushReq | Easy-Medium |
| simplebankedmemory | 1 | AddressConverter, Storage | Banks with pipelines | Medium |
| addresstranslator | 1 | 2× AddressToPortMapper | Transactions | Easy-Medium |
| datamover | 1 | 2× AddressToPortMapper | Inflight reqs | Easy-Medium |
| mmu | 1 | PageTable (external) | Translations, migration | Medium |
| switch | 1 | RoutingTable, Arbiter | Port mappings, buffers | Medium |
| endpoint | 1 | None significant | Msg assembly/disassembly | Easy-Medium |
| writearound cache | 1 (monolithic) | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory, MSHR | Hard |
| writeevict cache | 1 (monolithic) | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory, MSHR | Hard |
| writethrough cache | 1 (monolithic) | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory, MSHR | Hard |
| writeback cache | 1 (monolithic, 6 stages) | Directory, MSHR, Storage, AddrToPortMapper, VictimFinder | Huge state (60+ fields) | Very Hard |
| DRAM | 1 | AddrConverter, SubTransSplitter, AddrMapper, CmdQueue, Channel, Banks | Banks, channels, queues | Hard |
| tickingping (example) | 1 | None | Simple | Trivial |
| ping (example) | N/A | N/A | Not modeling.Component | N/A |

### M13: TLB — Comp Elimination + A-B State
- Eliminate Comp wrapper (or reduce to thin type)
- Migrate all Comp runtime fields into State (sets, MSHR, pipeline, buffer, respondingMSHREntry, inflightFlushReq, state string)
- The `state string` field on Comp → already in State as `TLBState`
- Inline AddressToPortMapper logic (port name in Spec, resolve via GetPortByName)
- Middleware reads A buffer, writes B buffer
- Remove snapshot/restore conversion layer (State becomes canonical)
- **Budget**: 5 cycles
- **Status**: NEXT

### M14: Simple Components Batch — mmuCache + addresstranslator + datamover + simplebankedmemory
- These are smaller/simpler components that can be done together
- Same pattern: eliminate Comp, inline deps, A-B state, State canonical
- **Budget**: 6 cycles

### M15: MMU + Switch + Endpoint
- mmu has PageTable as external dep (similar to Storage — external service)
- switch has RoutingTable + Arbiter (need careful analysis)
- endpoint has msg assembly logic
- **Budget**: 6 cycles

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

## Summary Statistics
- Total milestones completed: 12
- PRs merged: 32
- Components ported: 16/16
- Components fully transformed (Comp eliminated + A-B): 1/16

## Lessons Learned
- CI can get stuck in "queued" state — don't waste cycles waiting for it
- Architecture discussions should be fully resolved before implementation
- Multi-worker mechanical changes work well with clear patterns
- Breaking milestones to 2-6 cycle budgets is optimal
- Human feedback drives direction — stay responsive
- Combined milestones work when scope is small
- idealmemcontroller is the reference implementation — follow its patterns
- The snapshot/restore conversion layer disappears when State is canonical (big code reduction)
- A-B state deep copy via JSON round-trip is acceptable for small States
