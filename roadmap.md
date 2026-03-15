# Roadmap

## Project Goal

Evolve Akita V5 toward a clean, high-performance simulation framework with broad DRAM support, unified protocols, modern visualization, and clean architecture.

## Current State (Cycle 385)

### Previous Phase: ✅ COMPLETE (M1-M50)

All 50 milestones from the original component model refactoring are complete. All 16 success criteria met. CI green on main.

### Research Phase: ✅ RESEARCH COMPLETE

All 13 human topics have been researched. Findings summarized below. Human gave green light on: #477, #478, #479, #480, #483, #486, #487.

### New Topic: Integer ID (#501)
Human raised new discussion topic: replace string-based IDs with integer-based IDs to reduce GC/allocation overhead. Research complete — feasible, ~30-40 files, no blockers.

### Current Phase: Phase 2 Implementation

**M51**: ✅ COMPLETE — Double buffering residue cleanup (3 cycles budgeted, 3 used)
**M52**: ✅ COMPLETE — Component-engine decoupling (4 cycles budgeted, ~4 used)
**M53**: 🔄 NEXT — /mem/mem flattening (#486)

---

## Phase 2: Research Findings & Recommendations

### Topic 1: Move directconnection to noc? (#477)
**Recommendation: YES** — `noc/networkconnector` already type-asserts on `*directconnection.Comp`. Moving it is organizational improvement. 10 files need import updates. No circular dependency risk.

### Topic 2: Component-engine decoupling (#478) ✅ DONE (M52)
**Recommendation: YES** — Components only use `Schedule()` and `CurrentTime()`, never `Run()`/`Pause()`. Implemented in M52.

### Topic 3: Event serialization (#479)
**Recommendation: Replace `Handler()` with `HandlerID() string` + `HandlerRegistry`** — Events store handler name instead of pointer. Engine looks up handler at dispatch. Enables arbitrary checkpoint/restore without quiescence.

### Topic 4: Integer time representation (#480)
**Recommendation: YES, uint64 picoseconds** — Every major simulator uses integer time. `Freq` methods currently use `math.Round` hacks as precision workarounds. 1 GHz = 1000 ps (exact). Migration scope: ~21 core files, compiler catches all breakage.

### Topic 5: Merge concrete simulators into Akita? (#481)
**Recommendation: NO** — Keep separate repos.

### Topic 6: Merge AkitaRTM and Daisen? (#482)
**Recommendation: PARTIAL MERGE** — Unified frontend with live/replay modes.

### Topic 7: Double buffering residue (#483) ✅ DONE (M51)

### Topic 8: DRAM controller improvements (#484)
**Recommendation: Hybrid approach** — Build timing/scheduling natively in Go.

### Topic 9: Remove idealmemcontroller? (#485)
**Recommendation: NO, keep it.**

### Topic 10: /mem/mem → /mem flattening (#486)
**Recommendation: YES** — Parent `mem/` has zero Go source files. `mem/mem` stutters. 63 files need mechanical import change.

### Topic 11: Unified memory control protocol (#487)
**Recommendation: YES** — 3 incompatible patterns exist. Replace with single `ControlReq`/`ControlRsp` using enum commands.

### Topic 12: Rewrite Daisen/AkitaRTM with React (#488)
**Recommendation: YES, React** — Current frontends use vanilla TS+D3.

### Topic 13: No implementation without authorization (#489)
**Status: Acknowledged.**

### Topic 14: Integer ID representation (#501)
**Status: RESEARCH COMPLETE** — Feasible, ~30-40 files, no blockers. Main design decision: tracing composite IDs. Recommend uint64 for core IDs, convert to string at tracing boundary.

---

## Proposed Implementation Order

Based on dependency analysis and human authorization (green light on: #477, #478, #479, #480, #486, #487):

### Tier 1: Foundational changes
1. ~~**Double buffering cleanup** (#483)~~ — ✅ M51
2. ~~**Component-engine decoupling** (#478)~~ — ✅ M52
3. **/mem/mem flattening** (#486) — M53 (NEXT)
4. **Move directconnection to noc** (#477) — M54
5. **Integer time** (#480) — M55
6. **Event serialization** (#479) — M56

### Tier 2: Architecture improvements
7. **Integer ID** (#501) — Pending human authorization
8. **Unified control protocol** (#487) — M57+
9. **DRAM controller improvements** (#484) — Pending human authorization

### Tier 3: Frontend/tooling (independent track)
10. **Partial merge AkitaRTM/Daisen** (#482) — No authorization yet
11. **React rewrite** (#488) — No authorization yet

---

## Phase 2: Implementation Milestones

### M51: Double buffering residue cleanup (#483) ✅
- **Status**: COMPLETE (cycle 377-380, 3 cycles)
- **PR**: #85, merged

### M52: Component-engine decoupling (#478) ✅
- **Status**: COMPLETE (cycle 381-385, ~4 cycles)
- **PR**: #86, merged

### M53: /mem/mem flattening (#486)
- **Status**: NEXT
- **Budget**: 4 cycles
- **Scope**: Move Go files from `v5/mem/mem/` to `v5/mem/`, update ~63 import paths. Remove empty `mem/mem/` directory. All tests must pass. CI must pass.

### M54: Move directconnection to noc (#477)
- **Status**: Planned
- **Budget**: 3 cycles
- **Scope**: Move `v5/sim/directconnection/` to `v5/noc/directconnection/`, update ~20 import paths

### M55: Integer time (#480)
- **Status**: Planned
- **Budget**: 6 cycles
- **Scope**: Replace float64 `sim.VTimeInSec` with uint64 picoseconds. ~21 core files + downstream.

### M56: Event serialization (#479)
- **Status**: Planned
- **Budget**: 5 cycles
- **Scope**: Replace Handler() with HandlerID() + HandlerRegistry

### M57+: Remaining topics (unified control protocol, DRAM, etc.)
- **Status**: Planned — details to be refined

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
| Phase 6 (M40) | Default spec, rename, freq in spec | 8 | ~4 |
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
- Research across 13 topics completed efficiently in 1 cycle with 5 parallel workers
- M52 completed smoothly in ~4 cycles — mechanical refactors are predictable
