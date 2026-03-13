# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 309)

### Project Status: IN PROGRESS — New human requirements (issues #387, #389, #398)

M40 completed (default spec, rename, freq in spec). New human requirements received:
1. **Event-driven component support** (#389) — need a modeling pattern for components that schedule events in the far future instead of ticking every cycle
2. **Deep performance evaluation** (#387) — compare against upstream, identify bottlenecks
3. **Fix duplicated CI runs** (#398) — workflow triggers on both push and pull_request causing double runs
4. **Verify test sizes unchanged** (#385) — ensure no acceptance test sizes were reduced

### Investigation Phase (Current)
Workers researching event-driven patterns, performance benchmarks, and test size verification. Next milestone will be defined based on findings.

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

### M41: Fix CI duplication + verify test sizes (estimated 2 cycles)
- Fix `.github/workflows/akita_test.yml` to only trigger `push` on main branch
- Verify and document that all test sizes match upstream
- Quick win, high confidence

### M42: Event-driven component support (estimated 5-8 cycles)
- Depends on Iris's research findings
- May involve creating `modeling.EventComponent[S,T]` or similar
- Must maintain save/load compatibility
- Scope TBD after research

### M43: Performance optimization (estimated 5-8 cycles)
- Depends on Diana's benchmarking findings
- Address any bottlenecks identified
- Must not regress correctness

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
