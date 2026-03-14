# Roadmap

## Project Goal

Evolve Akita V5 toward a clean, high-performance simulation framework with broad DRAM support, unified protocols, modern visualization, and clean architecture.

## Current State (Cycle 375)

### Previous Phase: ✅ COMPLETE (M1-M50)

All 50 milestones from the original component model refactoring are complete. All 16 success criteria met. CI green on main.

### New Phase: Discussion & Research

The human has raised 13 discussion/research topics (issues #477-#489) with explicit instruction: **no implementation without authorization**. We are in a research-only phase.

---

## Phase 2: Research & Discussion (Current)

### Human Topics to Discuss

| Issue | Topic | Research Assignee | Status |
|-------|-------|-------------------|--------|
| #477 | Move directconnection to noc? | Elena (#492) | 🔄 In Progress |
| #478 | Component-engine decoupling (event scheduler) | Iris (#491) | 🔄 In Progress |
| #479 | Event serialization | Iris (#491) | 🔄 In Progress |
| #480 | Integer time representation (uint64 vs float64) | Iris (#490) | 🔄 In Progress |
| #481 | Merge concrete simulators into Akita? | Otto (#499) | 🔄 In Progress |
| #482 | Merge AkitaRTM and Daisen? | Mara (#496) | 🔄 In Progress |
| #483 | Double buffering residue | Diana (#495) | 🔄 In Progress |
| #484 | Improve DRAM controller modeling | Diana (#494) | 🔄 In Progress |
| #485 | Remove idealmemcontroller? | Elena (#493) | 🔄 In Progress |
| #486 | /mem/mem → /mem folder flattening | Elena (#493) | 🔄 In Progress |
| #487 | Unified memory control protocol | Otto (#498) | 🔄 In Progress |
| #488 | Rewrite Daisen/AkitaRTM with React | Mara (#497) | 🔄 In Progress |
| #489 | Meta: No implementation yet | — | Noted |

### Next Steps

1. ✅ Assign research workers to all 13 topics
2. ⬜ Collect research findings
3. ⬜ Synthesize findings into recommendations
4. ⬜ Present recommendations to human for authorization
5. ⬜ Define implementation milestones based on human decisions

---

## Phase 1: Component Model Refactoring (COMPLETE)

<details>
<summary>50 milestones completed across ~212 cycles</summary>

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
| Phase 17 (M50) | Final documentation polish | 4 | ~2 |
</details>

---

## Lessons Learned

- Research phases need clear scope per worker — one worker, one focused topic
- Human direction can pivot rapidly — stay responsive
- Budget honestly — track actual vs estimated cycles
- Investigation cycles before implementation prevent scope misjudgments
- All 16 original success criteria were met with systematic execution
