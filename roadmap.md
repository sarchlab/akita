# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 328)

### Project Status: IN PROGRESS — M42 complete, M43 defined (consolidate stateutil into queueing + pipeline migration)

**M42 complete (cycle ~326):** Switch and endpoint simplification done. PR #71 merged. Switch reduced from 735→290 lines, endpoint from 585→519. All human issues #402-#406 resolved. CI green.

**New human directive #414:** Remove `stateutil` package, consolidate `Buffer[T]` and `Pipeline[T]` into `queueing`. Remove `Pop` and `PopTyped` methods.

**M43 defined:** Consolidate stateutil → queueing, remove Pop/PopTyped, migrate simplebankedmemory + TLB pipelines. See tbc-db issue #420.

### Recently Completed

#### M40: Rename simplecache, DefaultSpec, Freq in Spec (DONE — Cycle 309)
- Budget: 8 | Used: ~4
- PR #69 merged
- simplecache renamed to writethroughcache
- DefaultSpec added to all 13 builder files
- Freq moved into each component's Spec struct
- CI passing

#### M39.1: Merge PR #68 and final verification (DONE — Cycle 299)
- PR #68 merged, CI 5/5 green, all 16 success criteria verified

#### M39: Final cleanup and documentation update (DONE — Cycle 294)
- Budget: 3 | Used: 4

#### M38: Eliminate transaction conversion layers in caches (DONE — Cycle 288)
- Budget: 8 | Used: ~5

#### M37: Migrate writeback cache and switch to stateutil (DONE — Cycle 279)
- Budget: 5 | Used: 4

#### M36: Create stateutil package with generic Buffer[T] and Pipeline[T] (DONE — Cycle 271)
- Budget: 5 | Used: 3

#### M35: Cache unification — merge 3 simple caches (DONE — Cycle 265)
- Budget: 5 | Used: 3

### Human Directions — Status

#### Ultimate Goal (issue #342)
1. Single simulation-level save and load ✅
2. No per-component custom save/load functions ✅
3. Developers only implement middleware Tick functions ✅
4. No performance compromise — under verification (#387)

#### Serializable Buffers/Pipelines (issue #343) — DONE

#### Global State Manager (issue #326) — DEFERRED

#### Default Spec / Rename / Freq (issue #384) — DONE in M40

#### Event-Driven Components (issue #389) — UNDER RESEARCH
- Some components schedule events rather than ticking
- Need design for modeling.Component variant or alternative pattern
- Iris researching approaches

#### Performance Evaluation (issue #387) — UNDER INVESTIGATION
- Diana benchmarking current vs original v4

#### Test Size Verification (issue #385) — UNDER AUDIT
- Elena verifying all acceptance test sizes match upstream

#### Duplicated CI Runs (issue #398) — READY TO FIX
- CI workflow triggers on both `push` and `pull_request`
- Fix: restrict `push` to `branches: [main]` only

---

## Planned Milestones

### M41: Restore test sizes + fix CI duplication (DONE — Cycle 314)
- Budget: 2 | Used: 2
- PR #70 merged
- All test sizes restored to upstream values
- CI push trigger restricted to main branch
- All CI green

### M42: Switch and endpoint simplification (#402-#406) (DONE — Cycle ~326)
- Budget: 10 | Used: ~10
- PR #71 merged
- Switch 735→290 lines, endpoint 585→519 lines
- All human issues #402-#406 resolved
- CI green

### M43: Consolidate stateutil into queueing + pipeline migration (Budget: 8 cycles)
- Move Buffer[T] and Pipeline[T] from stateutil → queueing
- Remove Pop and PopTyped from Buffer (#414)
- Remove old queueing.Buffer interface and bufferImpl
- Delete stateutil package entirely
- Migrate simplebankedmemory hand-coded pipeline → queueing.Pipeline[T]
- Migrate TLB hand-coded pipeline → queueing.Pipeline[T]
- Update all imports

### M44: Repo-wide simplification — remaining items (estimated 5-8 cycles)
- Extract shared convertAddress
- Unify Flush/Restart message types
- Extract shared MSHR and LRU set ops
- Remove dead code
- Address remaining #408 items

### M45: NOC performance optimization (estimated 5-8 cycles)
- Verify performance improved from M42/M43/M44 simplification
- Target: bring NOC tests within 2x of v4 performance
- Fix remaining allocation bottlenecks

### M46: Event-driven component support (estimated 5-8 cycles)
- Design A or B from Iris's research (NOT timer-based — human rejected)
- Create `modeling.EventComponent[S,T]` or extend Component with event slots
- Must support save/load of pending events

### M47: Global state manager (deferred, estimated 3-5 cycles)
- Single-call save/load of entire simulation state
- Depends on all components being fully standardized

---

## Success Criteria Checklist

| # | Criterion | Status |
|---|-----------|--------|
| 1 | Simple, intuitive APIs | ✅ |
| 2 | All CI checks pass on main | ✅ (5/5 green) |
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
| 15 | component_guide.md reflects final arch | ✅ |
| 16 | Performance matches original | Under verification (#387) |

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M38) | CI, performance, cache unification, stateutil, serialization | ~35 | ~25 |
| Phase 5 (M39-M39.1) | Documentation | 5 | 5 |
| Phase 6 (M40) | Default spec, rename, freq | 8 | ~4 |
| **Total** | **40 milestones** | **~264** | **~169** |

### Upcoming Phases
| Phase | Milestones | Est. Budget |
|-------|-----------|-------------|
| Phase 7 (M41) | Restore test sizes + CI fix | 2 (done) |
| Phase 8 (M42) | Switch/endpoint simplification | 10 (done) |
| Phase 9 (M43) | Consolidate stateutil→queueing + pipeline migration | 8 |
| Phase 10 (M44) | Repo-wide simplification remaining | 5-8 |
| Phase 11 (M45) | Performance optimization | 5-8 |
| Phase 12 (M46) | Event-driven components | 5-8 |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- Analysis/design phases pay off by preventing wrong implementations
- A-B deep copy was 100% wasted — always verify assumptions before building infrastructure
- In-place state update is simpler AND faster
- Cache adapter wrappers are complex boilerplate — generic types eliminated them
- Performance is non-negotiable — every change must be measured
- Transaction serialization (M38) was the last major piece — flattening pointer fields eliminated all conversion layers
- Documentation verification caught fabricated code — always verify code snippets against actual source files
- Event-driven components represent a new architectural challenge — different from tick-driven model
- Elena's audit missed acceptance_test.py changes — auditing only Go source files is insufficient; must check test runners (Python scripts) too
- Human's switch feedback (#402-#406) shows we over-engineered the serialization layer — simplicity beats correctness of abstraction
- Performance research showed the copy-per-tick pattern is a fundamental bottleneck when state contains large slices — need architectural fix, not optimization
