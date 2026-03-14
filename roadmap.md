# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 360)

### Project Status: CRITICAL — CI RED on main since PR #74

**Regression:** PR #74 (M45.1: coalescer removal) introduced a panic in `writethroughcache/bottomparser.go` — `WriteDirtyMask` is nil when accessed during MSHR merge. All writeevict, writethrough, writearound, and virtualmem acceptance tests crash. This has been red for ~8 commits.

**Process failure:** PR #75 (M46: EventDrivenComponent) was merged on top of red CI. Human issue #462 escalates this — "Same problem happened again. Please reflect." This is the second time agents merged a PR without CI green.

**Root cause:** The intake stage copies `WriteReq.DirtyMask` as-is (nil when not set by caller). The old coalescer normalized this; the new flat transaction model does not. When `mergeMSHRData` accesses `trans.WriteDirtyMask[i]` for a write transaction with nil DirtyMask, it panics.

**M46 (event-driven components) was implemented and merged** (PR #75), building on the broken foundation. The event-driven code itself appears functional.

---

## Active/Planned Milestones

### M47: Fix mem_acceptance_test regression (IMMEDIATE — estimated 3 cycles)
- Fix nil WriteDirtyMask panic in writethroughcache
- Normalize DirtyMask in intake stage: when nil, create all-true mask matching Data length
- Verify all 8 acceptance test groups pass (idealmem, writeback x2, dram, writeevict, writethrough, writearound, virtualmem)
- Add process guard: teach agents to NEVER merge PRs when CI jobs fail

### M48: sim.Component consolidation (estimated 4-6 cycles)
- Evaluate whether modeling.Component should move into sim package
- Simplify sim.Component interface if possible
- Address issue #440

### M49: Performance optimizations (estimated 4-6 cycles)
- Address Diana's findings: endpoint state copy, flit serialization
- Buffer ring buffer pattern
- Tracing guards

### M50: Global state manager (deferred, estimated 3-5 cycles)
- Single-call save/load of entire simulation state

---

## Completed Milestones Summary

| Phase | Milestones | Budget | Used |
|-------|-----------|--------|------|
| Phase 1 (M1-M20) | Core model, porting, A-B state | ~160 | ~100 |
| Phase 2 (M21-M26) | Cleanup, multi-MW, docs | ~40 | ~29 |
| Phase 3 (M27-M29) | Code quality | ~16 | ~6 |
| Phase 4 (M30-M38) | CI, performance, cache unification, stateutil | ~35 | ~25 |
| Phase 5 (M39-M39.1) | Documentation | 5 | 5 |
| Phase 6 (M40) | Default spec, rename, freq | 8 | ~4 |
| Phase 7 (M41) | Restore test sizes + CI fix | 2 | 2 |
| Phase 8 (M42) | Switch/endpoint simplification | 10 | ~10 |
| Phase 9 (M43) | Consolidate stateutil→queueing | 8 | ~6 |
| Phase 10 (M44) | Repo-wide cleanup: dead code, shared utils | 6 | ~4 |
| Phase 11 (M45.1) | Remove analysis + coalescer | 8 | ~4 |
| Phase 12 (M45.2) | File paradigm standardization | 10 | ~8 |
| Phase 13 (M46) | Event-driven component support | 8 | ~4 |
| **Total** | **46 milestones** | **~316** | **~207** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- **CRITICAL LESSON (recurring):** Agents must NEVER merge PRs when any CI job is failing. mem_acceptance_test failure was missed because agents only checked unit tests, not the full CI pipeline. This has happened TWICE now.
- **Process fix needed:** Ares/Apollo must verify ALL CI jobs green before claiming milestone complete or merging PR. The `mem_acceptance_test` job was failing but was ignored.
- Human direction can pivot rapidly — stay responsive, don't over-plan
- In-place state update is simpler AND faster than deep copy
- Event-driven components represent a fundamental architectural challenge — different from tick-driven model
- Using investigator agents (Elena, Iris) to audit/design before coding milestones works well
- Large mechanical refactorings benefit from parallelizing across multiple workers
- Coalescer removal broke MSHR merge because DirtyMask was previously normalized by the coalescer
