# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 369)

### Project Status: All human issues closed. All success criteria met.

CI is green on main (last run all 5 jobs passed; latest push in progress). M49 merged — sim.Component slimmed (Handler removed), repo hygiene complete.

**All 16 success criteria from spec.md are satisfied:**
1. ✅ Simple, intuitive APIs (modeling.Component[S,T])
2. ✅ CI green on main
3. ✅ Component = Spec + State + Ports + Middleware + Hooks
4. ✅ No unnecessary Comp wrappers (4 justified remain)
5. ✅ No external dependency interfaces
6. ✅ Single simulation-level save/load
7. ✅ Developers only implement middleware Tick
8. ✅ All runtime data in State
9. ✅ No conversion layers
10. ✅ No restoreFromState/syncToState
11. ✅ No runtime copies of State
12. ✅ Save/load acceptance tests pass
13. ✅ All first-party components use modeling pattern
14. ✅ All components have multiple middlewares
15. ✅ component_guide.md exists (needs EventDrivenComponent update)
16. ✅ Performance at v4 parity (Diana's benchmarks)

**All human issues closed:** #389, #408, #439, #440, #462

**Remaining polish items (not blocking):**
- component_guide.md needs EventDrivenComponent section
- spec.md active issues list needs updating
- Performance: allocation optimizations possible (ring buffer, flit escape) but wall-clock already at parity

---

## Final Assessment Milestone

### M50: Final review and documentation polish (current — estimated 2 cycles)
- Update component_guide.md with EventDrivenComponent
- Update spec.md to reflect completed items
- Clean up any remaining minor issues
- Final CI verification

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
| Phase 16 (M49) | sim.Component cleanup + repo hygiene | 4 | ~2 |
| **Total** | **49 milestones** | **~324** | **~212** |

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
- All 16 success criteria can be met with systematic milestone-by-milestone execution
