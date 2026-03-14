# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 364)

### Project Status: CI GREEN — M47 complete

CI is green on main (all 5 jobs pass). M47 (fix nil WriteDirtyMask panic) is merged (PR #76).

**Completed recently:**
- M46: EventDrivenComponent support (PR #75) — modeling.EventDrivenComponent[S,T] with Design B (state-encoded timers)
- M47: Fix CI regression from coalescer removal (PR #76) — normalized nil DirtyMask in writethroughcache intake
- M45.2: File paradigm standardization — all 14 component packages now have spec.go + state.go + per-middleware files

**Key remaining human issues:**
- #462: PR merged with red CI — process reflection (addressed by enforcing all-5-CI-green rule)
- #440: sim.Component still exists alongside modeling.Component — consolidation needed
- #439: Component file paradigm (mostly done in M45.2)
- #408: Repo-wide simplification — wrappers, indirections, residues from old patterns
- #389: Event-driven components (done in M46)

---

## Active/Planned Milestones

### M48: Investigate sim.Component consolidation + repo simplification (current — investigation cycle)
- Iris analyzing feasibility of consolidating sim.Component with modeling.Component (#466)
- Elena auditing remaining simplification opportunities (#467)
- Will define concrete implementation milestone after investigation

### M49: [TBD — based on M48 investigation] (estimated 4-8 cycles)
- Likely: sim.Component consolidation + cleanup of leftover artifacts
- Address #440, #408

### M50: Performance optimizations (estimated 4-6 cycles)
- Ring buffer for queueing.Buffer (-36% allocations)
- Fix switch sendOut flit escape (-11% allocations)
- Guard tracing string creation (-9% allocations)
- Address Diana's findings from performance analysis

### M51: Global state manager (deferred, estimated 3-5 cycles)
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
| Phase 14 (M47) | Fix nil WriteDirtyMask CI regression | 3 | ~2 |
| **Total** | **47 milestones** | **~319** | **~209** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- **CRITICAL LESSON (recurring):** Agents must NEVER merge PRs when any CI job is failing. This has happened TWICE now. All 5 CI jobs must be GREEN before merge.
- Human direction can pivot rapidly — stay responsive, don't over-plan
- In-place state update is simpler AND faster than deep copy
- Event-driven components represent a fundamental architectural challenge — different from tick-driven model
- Using investigator agents (Elena, Iris, Diana) to audit/design before coding milestones works well
- Large mechanical refactorings benefit from parallelizing across multiple workers
- Coalescer removal broke MSHR merge because DirtyMask was previously normalized by the coalescer
- Investigation cycles (scheduling auditor agents before defining milestones) prevent scope misjudgments
