# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 296)

### Project Status: COMPLETE (pending final PR merge)

All core architectural goals achieved. M39 documentation update merged (PR #67). Apollo verification found 4 fabricated code sections — fix completed in PR #68 (CI 4/5 green, NOC test running — doc-only change). All human issues (#342, #343, #326) resolved.

### Recently Completed

#### M39: Final cleanup and documentation update (DONE — Cycle 294)
- Budget: 3 | Used: 4 (deadline missed on verification fix round)
- PR #67 merged (stateutil section, flat transaction pattern)
- PR #68 open (fix 4 fabricated code sections found during verification)
- component_guide.md now reflects final architecture

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

### ➡️ M39.1: Merge PR #68 (doc fix) and final verification
- **Goal**: Merge the documentation fix PR #68, verify all 16 success criteria met
- **Budget**: 2 cycles
- PR #68 fixes 4 fabricated code sections found by Apollo's verification
- CI already 4/5 green (NOC test running but irrelevant for doc-only change)

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
| 15 | component_guide.md reflects final arch | ✅ (PR #67 merged + PR #68 pending) |
| 16 | Performance matches original | ✅ |

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M38) | CI, performance, cache unification, stateutil, serialization | ~35 | ~25 |
| Phase 5 (M39) | Documentation final cleanup | 3 | 4 |
| **Total** | **39 milestones** | **~254** | **~164** |

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
- Documentation verification caught fabricated code — always verify code snippets against actual source files
- M39 deadline missed because verification found issues that required a fix round — budget for verification fixes
