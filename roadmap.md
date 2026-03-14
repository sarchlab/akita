# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 370)

### Project Status: Final documentation polish needed (M50)

CI is green on main (all 5 jobs pass). M49 merged. All 16 success criteria are functionally met, but criterion #15 (component_guide.md reflects architecture) has documentation accuracy issues that need fixing before declaring project complete.

**Success criteria status:**
1-14, 16: ✅ All met
15: ⚠️ component_guide.md exists but has stale `stateutil` references (29 occurrences, should be `queueing`) and missing EventDrivenComponent section

**Remaining issues found by final review (Iris #471, Elena #472):**
- component_guide.md: 29 stale `stateutil` → `queueing` references, missing EventDrivenComponent section, stale file paths
- README.md: broken link (`migration_guide.md` → `migration.md`)
- migration.md: references nonexistent `WithSimulation` API
- writebackcache binary (9.4MB) tracked in git
- gmmu mock_port.go deleted but needed locally (CI passes via go generate)
- 3 doc.go typos, missing doc.go for queueing/ and simulation/
- spec.md active issues list stale

**All human issues closed:** #389, #408, #439, #440, #462

**Remaining polish items (not blocking):**
- component_guide.md needs EventDrivenComponent section
- spec.md active issues list needs updating
- Performance: allocation optimizations possible (ring buffer, flit escape) but wall-clock already at parity

---

## Final Assessment Milestone

### M50: Final documentation polish and repo hygiene (IN PROGRESS — budget 4 cycles)
Fix all documentation issues found in final review:
1. **component_guide.md**: Replace all 29 `stateutil` references with `queueing`, add EventDrivenComponent section, fix stale file paths, fix Spec struct / builder discrepancies, remove Pop/PopTyped references
2. **README.md**: Fix broken `migration_guide.md` link → `migration.md`  
3. **migration.md**: Fix `WithSimulation` API reference to actual builder API
4. **Repo hygiene**: Remove tracked `writebackcache` binary, fix gmmu mock_port.go (rename to `_test.go` pattern)
5. **doc.go**: Fix 3 typos (memoy, fix→fixed, infrascturctures), add doc.go for `queueing/` and `simulation/`
6. **spec.md**: Move completed items from Active to Resolved

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
