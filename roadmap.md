# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 279)

### Recently Completed

#### M37: Migrate writeback cache and switch to stateutil (DONE — Cycle 279)
- Budget: 5 | Used: 4
- PR #66 merged: writeback adapters.go deleted, switch thin adapters use dynamic buf()
- All stateutil.Buffer[T] and Pipeline[T] used everywhere
- Zero adapter types remain in codebase
- Verified by Apollo, all 5 CI checks pass

#### M36: Create stateutil package with generic Buffer[T] and Pipeline[T] (DONE — Cycle 271)
- Budget: 5 | Used: 3
- PR #65 merged: stateutil package created, simplecache migrated

#### M35: Cache unification — merge 3 simple caches (DONE — Cycle 265)
- Budget: 5 | Used: 3
- PR #64 merged: writearound/writeevict/writethrough → unified simplecache with WritePolicy strategy

### Active Human Directions

#### Ultimate Goal (issue #342)
1. Single simulation-level save and load ✅
2. No per-component custom save/load functions — **IN PROGRESS** (caches still have conversion layers)
3. Developers only implement middleware Tick functions — **IN PROGRESS** (caches need GetState/SetState)
4. No performance compromise ✅

#### Serializable Buffers/Pipelines (issue #343) — LARGELY DONE
- stateutil.Buffer[T] and Pipeline[T] created and used everywhere
- Remaining issue: cache transaction types still use pointers, requiring snapshot/restore layers

#### Global State Manager (issue #326) — DEFERRED
- Not practical as primary access path (75× perf penalty)
- May revisit after core migration complete

---

## Next Milestones

### ➡️ M38: Eliminate transaction conversion layers in caches
- **Goal**: Make transactionState fully serializable in both simplecache and writeback, eliminating all snapshot/restore conversion code
- **Issue**: #364
- **Scope**:
  - Replace `*mem.ReadReq`, `*mem.WriteReq`, `*cache.FlushReq` pointer fields with serializable value fields
  - Replace `[]*transactionState` cross-references with `[]int` indices
  - Delete state.go conversion functions (~670 lines total: 278 simplecache + 391 writeback)
  - Remove custom GetState/SetState from middleware
  - Reconstruct concrete message types only at send boundaries
  - All tests + CI green, save/load acceptance test passes
- **Budget**: 8 cycles (complex refactor touching many files in both cache packages)

### M39: Final cleanup and component guide update
- Eliminate switch thin adapter wrappers (forwardBufAdapter, sendOutBufAdapter) if possible
- Update component_guide.md to reflect final architecture
- Verify all remaining components have no custom save/load boilerplate
- Budget: 3 cycles

### M40: Project completion verification
- Full audit: every component uses only Spec + State + Ports + Middleware + Hooks
- No boilerplate beyond middleware Tick functions
- Save/load works for entire simulation automatically
- Performance benchmarks match original akita
- Budget: 2 cycles

---

## What's Done

| Category | Status | Details |
|----------|--------|---------|
| modeling package | ✅ | `modeling.Component[Spec, State]` with in-place state update |
| Messages as concrete types | ✅ | All messages are plain structs with `sim.MsgMeta` |
| Save/Load | ✅ | `simulation.Save/Load` works, acceptance test passes |
| 16 components ported | ✅ | All use `modeling.Component[Spec, State]` |
| MSHR/Directory as State + free functions | ✅ | Shared ops in `mem/cache/`, indices instead of pointers |
| stateutil package | ✅ | Generic Buffer[T] and Pipeline[T], JSON-serializable |
| All components use stateutil | ✅ | simplecache, writeback, switch all migrated (M36+M37) |
| Dependencies inlined | ✅ | All internal packages eliminated |
| In-place state update | ✅ | No A-B deep copy, 0µs overhead (M34) |
| Comp wrapper elimination | ✅ | Only thin wrappers remain for StorageOwner/API |
| Multi-MW split (all 16 components) | ✅ | 2-3 middlewares each |
| Cache unification | ✅ | 3 simple caches → simplecache with WritePolicy (M35) |
| CI passing | ✅ | All jobs pass |
| Performance parity | ✅ | 0µs overhead vs original akita |

### Remaining Conversion Code
| Package | Conversion Lines | Status |
|---------|-----------------|--------|
| simplecache/state.go | 278 | ⬜ M38 |
| writeback/state.go | 391 | ⬜ M38 |
| switch thin adapters | ~120 | ⬜ M39 |

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M37) | CI, performance, cache unification, stateutil | ~27 | ~20 |
| **Total** | **37 milestones** | **~243** | **~155** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- Analysis/design phases (Diana, Iris) pay off by preventing wrong implementations
- A-B deep copy was 100% wasted — always verify assumptions before building infrastructure
- In-place state update is simpler AND faster
- Cache adapter wrappers are complex boilerplate — generic types eliminated them
- Performance is non-negotiable — every change must be measured
- Human prefers discussion before coding for architectural decisions
- Cache unification (M35) went smoothly because design was thoroughly analyzed first
- stateutil migration (M36+M37) completed quickly with focused workers
- Transaction conversion layers are the last major boilerplate — making transaction types directly serializable is the final push
