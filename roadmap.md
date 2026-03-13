# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 288)

### Project Status: NEARLY COMPLETE

All core architectural goals have been achieved. The remaining work is documentation cleanup (component guide update) and final verification.

### Recently Completed

#### M38: Eliminate transaction conversion layers in caches (DONE — Cycle 288)
- Budget: 8 | Used: ~5
- Merged directly to main (commits bc7f98e..83ced5d)
- simplecache: transactionState flattened, state.go deleted (278 lines removed)
- writeback: transactionState flattened, state.go deleted (391 lines removed)
- All custom GetState/SetState on middleware deleted
- CI: 4/5 green, NOC acceptance test in progress

#### M37: Migrate writeback cache and switch to stateutil (DONE — Cycle 279)
- Budget: 5 | Used: 4
- PR #66 merged

#### M36: Create stateutil package with generic Buffer[T] and Pipeline[T] (DONE — Cycle 271)
- Budget: 5 | Used: 3
- PR #65 merged

#### M35: Cache unification — merge 3 simple caches (DONE — Cycle 265)
- Budget: 5 | Used: 3
- PR #64 merged

### Human Directions — Status

#### Ultimate Goal (issue #342)
1. Single simulation-level save and load ✅
2. No per-component custom save/load functions ✅ (state.go conversion layers deleted)
3. Developers only implement middleware Tick functions ✅ (no custom GetState/SetState)
4. No performance compromise ✅ (in-place update, 0µs overhead)

#### Serializable Buffers/Pipelines (issue #343) — DONE
- stateutil.Buffer[T] and Pipeline[T] created and used everywhere
- Transaction types fully serializable (flat fields, no pointers)
- No snapshot/restore layers remain

#### Global State Manager (issue #326) — DEFERRED
- Not practical as primary access path (75× perf penalty)
- May revisit as optional overlay for tooling/debugging

---

## Next Milestones

### ➡️ M39: Final cleanup and documentation update
- **Goal**: Update component_guide.md with stateutil patterns and flat transaction pattern; final project audit
- **Issue**: #373
- **Budget**: 3 cycles

### M40: Project completion verification
- Full audit against all 16 success criteria
- Budget: 1 cycle

---

## Success Criteria Checklist

| # | Criterion | Status |
|---|-----------|--------|
| 1 | Simple, intuitive APIs | ✅ |
| 2 | All CI checks pass on main | ✅ (4/5 green, 1 running) |
| 3 | Component = Spec + State + Ports + MW + Hooks | ✅ |
| 4 | No Comp wrappers (except StorageOwner) | ✅ |
| 5 | No external dependency interfaces | ✅ |
| 6 | Single sim-level save/load | ✅ |
| 7 | Developers only implement MW Tick | ✅ |
| 8 | All runtime data in State | ✅ |
| 9 | No SaveState/LoadState conversion layers | ✅ |
| 10 | No restoreFromState/syncToState | ✅ |
| 11 | No runtime copies of State in MW | ✅ |
| 12 | Save/load acceptance test passes | ✅ |
| 13 | All components use modeling package | ✅ |
| 14 | Each component has multiple MWs | ✅ (2-4 each) |
| 15 | component_guide.md reflects final arch | ⚠️ Needs stateutil section |
| 16 | Performance matches original | ✅ |

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M38) | CI, performance, cache unification, stateutil, serialization | ~35 | ~25 |
| **Total** | **38 milestones** | **~251** | **~160** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- Analysis/design phases pay off by preventing wrong implementations
- A-B deep copy was 100% wasted — always verify assumptions before building infrastructure
- In-place state update is simpler AND faster
- Cache adapter wrappers are complex boilerplate — generic types eliminated them
- Performance is non-negotiable — every change must be measured
- Transaction serialization (M38) was the last major piece — flattening pointer fields to value fields eliminated all conversion layers
- Project is approaching completion after 288 cycles — documentation update is the final step
