# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 271)

### Completed This Session

#### M36: Create stateutil package with generic Buffer[T] and Pipeline[T] (DONE — Cycle 271)
- Budget: 5 | Used: 3
- PR #65 merged: stateutil package created, simplecache migrated
- ~470 lines adapter code eliminated from simplecache
- Buffer[T] and Pipeline[T] are JSON-serializable value types
- Verified by Apollo, all tests pass

#### M35: Cache unification — merge 3 simple caches (DONE — Cycle 265)
- Budget: 5 | Used: 3
- PR #64 merged: writearound/writeevict/writethrough → unified simplecache with WritePolicy strategy
- ~9,500 lines eliminated

### Active Human Directions

#### Ultimate Goal (issue #342)
1. Single simulation-level save and load
2. No per-component custom save/load functions
3. Developers only implement middleware Tick functions
4. No performance compromise

#### Serializable Buffers/Pipelines (issue #343)
- stateutil package now provides Buffer[T] and Pipeline[T]
- Simplecache fully migrated (M36)
- Writeback and switch still use legacy adapters — M37 will migrate them

#### Global State Manager (issue #326) — DEFERRED
- String-based state registry for tooling/debugging
- Not practical as primary access path (75× perf penalty)
- May revisit after core migration complete

---

## Next Milestones

### ➡️ M37: Migrate writeback cache and switch to stateutil (READY)
- **Goal**: Eliminate all remaining adapter boilerplate by migrating writeback and switch to stateutil.Buffer[T] / Pipeline[T]
- **Issue**: #358
- **Scope**:
  - Writeback: Delete adapters.go (453 lines), replace pipeline free functions (~350 lines) with stateutil.Pipeline[int], update all middleware call sites
  - Switch: Replace stateFlitBuffer with stateutil.Buffer[flitPipelineItemState], handle ForwardBuffer/SendOutBuffer resolution
  - Delete all updateAdapterPointers() functions
  - All tests + CI green
- **Budget**: 5 cycles
- **Expected outcome**: Zero adapter types remain in entire codebase

### M38: Final cleanup — eliminate all custom save/load boilerplate
- Verify save/load acceptance test works end-to-end with stateutil types
- Remove any remaining snapshot/restore conversion code
- Ensure no component needs custom save/load beyond modeling.Component's SaveState/LoadState
- Update component_guide.md to reflect final architecture
- Budget: 3 cycles

### M39: Project completion verification
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
| simplecache uses stateutil | ✅ | No adapters, direct stateutil types in State (M36) |
| Dependencies inlined | ✅ | All internal packages eliminated |
| In-place state update | ✅ | No A-B deep copy, 0µs overhead (M34) |
| Comp wrapper elimination | ✅ | Only thin wrappers remain for StorageOwner/API |
| Multi-MW split (all 16 components) | ✅ | 2-3 middlewares each |
| Cache unification | ✅ | 3 simple caches → simplecache with WritePolicy (M35) |
| CI passing | ✅ | All jobs pass |
| Performance parity | ✅ | 0µs overhead vs original akita |

### Remaining Adapter Code
| Package | Adapter Lines | Status |
|---------|--------------|--------|
| simplecache | 0 | ✅ Migrated (M36) |
| writeback | ~800 | ⬜ M37 |
| switch | ~130 | ⬜ M37 |

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M36) | CI, performance, cache unification, stateutil | ~22 | ~16 |
| **Total** | **36 milestones** | **~238** | **~151** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- Analysis/design phases (Diana, Iris) pay off by preventing wrong implementations
- A-B deep copy was 100% wasted — always verify assumptions before building infrastructure
- In-place state update is simpler AND faster
- Cache adapter wrappers are complex boilerplate — generic types will eliminate them
- Performance is non-negotiable — every change must be measured
- Human prefers discussion before coding for architectural decisions
- Cache unification (M35) went smoothly because design was thoroughly analyzed first
- stateutil migration (M36) completed quickly because simplecache was fresh from M35 unification
