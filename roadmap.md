# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 266)

### Completed This Session

#### M35: Cache unification — merge 3 simple caches (DONE — Cycle 265)
- Budget: 5 | Used: 3
- PR #64 merged: writearound/writeevict/writethrough → unified simplecache with WritePolicy strategy
- ~9,500 lines eliminated
- WritePolicy interface with 3 implementations: WritearoundPolicy, WriteevictPolicy, WritethroughPolicy
- Verified by Apollo, all tests pass

### Active Human Directions

#### Ultimate Goal (issue #342)
1. Single simulation-level save and load
2. No per-component custom save/load functions
3. Developers only implement middleware Tick functions
4. No performance compromise

#### Serializable Buffers/Pipelines (issue #343)
- Human wants buffers implementing serialize interface as state members
- Iris completed thorough design analysis (#348)
- Proposed: generic `Buffer[T]` and `Pipeline[T]` in `stateutil` package
- Key insight: no serialize interface needed — value types with JSON tags serialize automatically

#### Global State Manager (issue #326) — DEFERRED
- String-based state registry for tooling/debugging
- Not practical as primary access path (75× perf penalty)
- Depends on future direction

---

## Next Milestones

### ➡️ M36: Create stateutil package with generic Buffer[T] and Pipeline[T] (READY)
- **Goal**: Create `stateutil` package and migrate simplecache to use it (eliminating adapter boilerplate)
- **Human direction**: issue #343 (serializable buffers/pipelines)
- **Design**: Iris's analysis (issue #348, workspace/iris/note.md)
- **Scope**:
  1. Create `v5/stateutil/` package with `Buffer[T]` and `Pipeline[T]` generic types
  2. Migrate simplecache to use `stateutil.Buffer[T]` and `stateutil.Pipeline[T]` — delete adapters.go
  3. All tests must pass, CI green
- **Budget**: 5 cycles
- **Expected outcome**: ~470 lines adapter code eliminated from simplecache, stateutil reusable for writeback + switch

### M37: Migrate writeback cache and switch to stateutil
- Migrate writeback adapters.go (~453 lines) → stateutil.Buffer[T] / Pipeline[T]
- Migrate switch adapter code (~130 lines) → stateutil types
- Delete updateAdapterPointers from all components
- Budget: 5 cycles

### M38: Final cleanup — eliminate all custom save/load boilerplate
- Verify save/load acceptance test works with new stateutil types
- Remove any remaining snapshot/restore conversion code that stateutil obsoletes
- Ensure no component needs custom save/load beyond modeling.Component's SaveState/LoadState
- Update component_guide.md
- Budget: 3 cycles

---

## What's Done

| Category | Status | Details |
|----------|--------|---------|
| modeling package | ✅ | `modeling.Component[Spec, State]` with in-place state update |
| Messages as concrete types | ✅ | All messages are plain structs with `sim.MsgMeta` |
| Save/Load | ✅ | `simulation.Save/Load` works, acceptance test passes |
| 16 components ported | ✅ | All use `modeling.Component[Spec, State]` |
| MSHR/Directory as State + free functions | ✅ | Shared ops in `mem/cache/`, indices instead of pointers |
| Pipeline/Buffer as State (caches + switch) | ✅ | Adapters pattern — to be replaced by stateutil |
| Dependencies inlined | ✅ | All internal packages eliminated |
| In-place state update | ✅ | No A-B deep copy, 0µs overhead (M34) |
| Comp wrapper elimination | ✅ | Only thin wrappers remain for StorageOwner/API |
| Multi-MW split (all 16 components) | ✅ | 2-3 middlewares each |
| Cache unification | ✅ | 3 simple caches → simplecache with WritePolicy (M35) |
| CI passing | ✅ | All jobs pass |
| Performance parity | ✅ | 0µs overhead vs original akita |

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M35) | CI, performance, cache unification | ~17 | ~13 |
| **Total** | **35 milestones** | **~233** | **~148** |

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
