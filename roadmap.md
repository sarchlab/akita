# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 365)

### Project Status: CI GREEN — M48 investigation complete

CI is green on main (all 5 jobs pass). M48 investigation cycle complete — Iris analyzed sim.Component consolidation (#466), Elena audited repo simplification opportunities (#467).

**Investigation findings (M48):**
- Iris: sim.Component interface cannot be eliminated (used by ports, simulation, monitoring). Recommends Design 2: remove Handler from interface + add modeling/doc.go. Low risk, ~20 lines.
- Elena: File paradigm 100% done. No double-buffering residues. Remaining cleanup: untrack mock_port.go, fix swtich_test.go typo, fix monitoring dead code, clean artifacts.
- Remaining Comp wrappers (idealmemcontroller, simplebankedmemory, endpoint) all justified — no further action.

**Human issues status:**
- #462: PR merged with red CI — process fixed, CI now green
- #440: sim.Component — investigation done, Design 2 selected for M49
- #439: Component file paradigm — DONE in M45.2
- #408: Repo simplification — audit done, actionable items in M49
- #389: Event-driven components — DONE in M46

---

## Active/Planned Milestones

### M48: Investigate sim.Component consolidation + repo simplification ✅ (1 cycle)
- Iris: Design 2 recommended — slim interface + docs (issue #466)
- Elena: Full audit complete, actionable items identified (issue #467)

### M49: sim.Component cleanup + repo hygiene (current — estimated 4 cycles)
- Remove Handler from sim.Component interface
- Add modeling/doc.go explaining sim/modeling split
- Untrack mock_port.go, fix filename typo, fix monitoring dead code
- Addresses human issues #440 and #408

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
| Phase 15 (M48) | Investigation: sim.Component + simplification | 1 | 1 |
| **Total** | **48 milestones** | **~320** | **~210** |

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
