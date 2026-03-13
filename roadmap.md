# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 262)

### M34: Eliminate A-B deep copy + revert NOC test sizes (DONE — Cycle 261)
- Budget: 4 | Used: 2
- PR #63 merged: Switched from deep-copy A-B buffering to in-place state update
- Removed all deepCopy/reflectDeepCopy functions (~150 lines removed)
- Reverted NOC test sizes to original upstream values
- Updated component_guide.md for in-place semantics
- CI: Branch CI passed. Main CI in progress.
- **Performance: 0µs deep copy overhead per tick (was 19µs with reflect, 322µs with gob)**

### New Human Directions (Cycle 262)

#### Ultimate Goal Clarification (issue #342)
1. Single simulation-level save and load
2. No per-component custom save/load functions
3. Developers only implement middleware Tick functions
4. No performance compromise

#### Buffers/Pipelines in State (issue #343)
- Human wants buffers to implement a serialize interface so they can be state members
- Need to discuss how to handle pipelines
- This would eliminate adapter wrappers (like stateTransBuffer in writeback cache)

#### Cache Unification APPROVED (issue #336)
- Human explicitly approved: "Let's go with merging the 3 simpler caches"
- 3 caches (writearound, writeevict, writethrough) → 1 unified cache with WritePolicy strategy
- Writeback stays separate
- ~5,300 lines eliminated

---

## Next Milestones

### ➡️ M35: Cache unification — merge 3 simple caches (READY)
- **Goal**: Merge writearound/writeevict/writethrough into a single unified cache component with a WritePolicy strategy interface
- **Human approval**: issue #336 comment "Let's go with merging the 3 simpler caches"
- **Design**: Iris's analysis (issue #336, workspace/iris/note.md) — WritePolicy strategy with 3 methods: HandleWriteHit, HandleWriteMiss, NeedsDualCompletion
- **Budget**: 5 cycles
- **Expected outcome**: ~5,300 lines eliminated, one cache package replaces 3

### M36: Serializable buffers/pipelines in state (NEEDS DISCUSSION)
- **Goal**: Design and implement a serialization interface for buffers and pipelines so they can be first-class state members
- **Human direction**: issue #343
- **Questions to resolve**:
  - What serialization interface? (Serialize/Deserialize methods? encoding.BinaryMarshaler?)
  - How do pipelines work as state? (Pipeline stages as typed slices with Push/Tick free functions?)
  - How does this interact with save/load? (gob registration? JSON? custom codec?)
  - Should adapter wrappers (stateTransBuffer, stateFlitBuffer, etc.) be replaced?
- **Status**: Needs design discussion before implementation
- **Budget**: TBD (discussion + implementation)

### M37: Global state manager (LONG TERM, DEFERRED)
- String-based state registry for tooling/debugging
- NOT as primary access path (75× performance penalty)
- Depends on human direction

---

## What's Done

| Category | Status | Details |
|----------|--------|---------|
| modeling package | ✅ | `modeling.Component[Spec, State]` with in-place state update |
| Messages as concrete types | ✅ | All messages are plain structs with `sim.MsgMeta` |
| Save/Load | ✅ | `simulation.Save/Load` works, acceptance test passes |
| 16 components ported | ✅ | All use `modeling.Component[Spec, State]` |
| MSHR/Directory as State + free functions | ✅ | Shared ops in `mem/cache/`, indices instead of pointers |
| Pipeline/Buffer as State (caches + switch) | ✅ | `queueing.Pipeline/Buffer` eliminated in caches/switch, adapters.go pattern |
| Dependencies inlined (DRAM) | ✅ | All internal packages eliminated, logic embedded |
| Dependencies inlined (caches) | ✅ | legacyMapper resolved at Build time, routing via Spec |
| In-place state update | ✅ | No A-B deep copy, 0µs overhead (M34) |
| Comp wrapper elimination | ✅ | Only thin wrappers remain for StorageOwner/API |
| Middleware boilerplate eliminated | ✅ | All delegation methods removed |
| Multi-MW split (all 16 components) | ✅ | 2-3 middlewares each |
| CI passing | ✅ | All jobs pass |
| Performance parity | ✅ | 0µs overhead vs original akita's 0µs |
| NOC test sizes | ✅ | Reverted to upstream values |

### Per-Component Status

| Component | Multi-MW | MW Count | Notes |
|-----------|:---:|:---:|---|
| idealmemcontroller | ✅ | 2 | thin Comp (StorageOwner) |
| writearound cache | ✅ | 2 | To be merged in M35 |
| writeevict cache | ✅ | 2 | To be merged in M35 |
| writethrough cache | ✅ | 2 | To be merged in M35 |
| writeback cache | ✅ | 2 | Stays separate |
| TLB | ✅ | 2 | |
| mmuCache | ✅ | 2 | |
| DRAM | ✅ | 3 | |
| addresstranslator | ✅ | 2 | |
| datamover | ✅ | 2 | |
| simplebankedmemory | ✅ | 2 | thin Comp (StorageOwner) |
| MMU | ✅ | 2 | |
| GMMU | ✅ | 2 | |
| endpoint | ✅ | 2 | thin Comp (API) |
| switch | ✅ | 2 | thin Comp (API) |
| tickingping | ✅ | 2 | |

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M34) | CI, performance | ~12 | ~10 |
| **Total** | **34 milestones** | **~228** | **~145** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- Analysis/design phases (Diana, Iris, Elena) pay off by preventing wrong implementations
- A-B deep copy was 100% wasted — no component actually used isolation. Always verify assumptions before building infrastructure.
- In-place state update is simpler AND faster — sometimes the simplest approach wins
- Cache adapter wrappers (stateTransBuffer, etc.) are complex boilerplate — serializable buffers could eliminate them
- Performance is non-negotiable for the human — every change must be measured
- Human prefers discussion before coding for architectural decisions
- Always check human constraints before defining scope (M28 revert, M31 runner issue)
