# Roadmap

## Project Goal

Evolve Akita V5 toward a clean, high-performance simulation framework with broad DRAM support, unified protocols, modern visualization, and clean architecture.

## Current State (Cycle 481)

### Previous Phase: ✅ COMPLETE (M1-M50)

All 50 milestones from the original component model refactoring are complete. All 16 success criteria met. CI green on main.

### Research Phase: ✅ RESEARCH COMPLETE

All 14 human topics have been researched. Findings summarized below. Human gave green light on: #477, #478, #479, #480, #483, #486, #487, #484, #501.

### Phase 2 Implementation: ✅ ALL AUTHORIZED WORK COMPLETE

**M51**: ✅ COMPLETE — Double buffering residue cleanup (3 cycles budgeted, 3 used)
**M52**: ✅ COMPLETE — Component-engine decoupling (4 cycles budgeted, ~4 used)
**M53**: ✅ COMPLETE — /mem/mem flattening (4 cycles budgeted, ~4 used)
**M54**: ✅ COMPLETE — Move directconnection to noc (#477) (3 cycles budgeted, ~3 used)
**M55**: ✅ COMPLETE — Integer time representation (#480) (6 cycles budgeted, ~4 used)
**M56**: ✅ COMPLETE — Event serialization (#479) (6 cycles budgeted, ~4 used)
**M57**: ✅ COMPLETE — Unified control protocol (#487) (6 cycles budgeted, ~4 used)
**M58**: ✅ COMPLETE — DRAM improvements phase 1: predefined specs, open-page policy, FR-FCFS scheduling (#484) (8 cycles budgeted, ~4 used)
**M59**: ✅ COMPLETE — Quality cleanup: serialization bug, spec validation, repo hygiene (4 cycles)
**M60**: ✅ COMPLETE — Fix CI lint + DRAM phase 2: validation, refresh, tFAW, statistics (4 cycles)
**M61**: ✅ COMPLETE — Integer ID migration: all entity IDs from string to uint64 (8 budgeted, ~5 used)
**M62**: ✅ COMPLETE — DRAM cross-validation against DRAMSim3/Ramulator2 (6 budgeted, ~4 used)

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
**Recommendation: FULL MERGE** — One Go server, one React frontend, one SQLite database. Live mode = simulation writes + frontend polls. Replay mode = read completed file. Human conceptually agrees.

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

### M53: /mem/mem flattening (#486) ✅
- **Status**: COMPLETE (cycle 385-389, ~4 cycles)
- **PR**: #87, merged
- **Scope**: Moved 12 Go files from `v5/mem/mem/` to `v5/mem/`, updated 65 import paths

### M54: Move directconnection to noc (#477) ✅
- **Status**: COMPLETE (cycle 390-394, ~3 cycles)
- **PR**: #88, merged
- **Scope**: Moved `v5/sim/directconnection/` to `v5/noc/directconnection/`, updated 22 import paths. Pure mechanical refactor.

### M55: Integer time (#480) ✅
- **Status**: COMPLETE (cycle 395-398, ~4 cycles)
- **PR**: #89, merged
- **Scope**: Replaced float64 `VTimeInSec` with uint64 picoseconds, `Freq` with uint64 Hz. Rewrote freq.go with integer arithmetic, eliminated all math.Round/Ceil/Floor hacks.

### M56: Event serialization (#479) ✅
- **Status**: COMPLETE (cycle 399-402, ~4 cycles)
- **PR**: #90, merged
- **Scope**: Replaced Handler() with HandlerID() string + HandlerRegistry. Made EventBase fully JSON-serializable. All event-creating code uses handler name strings. SerialEngine and ParallelEngine dispatch via registry lookup.

### M57: Unified control protocol (#487) ✅
- **Status**: COMPLETE (cycle 404-408, ~4 cycles)
- **PR**: #91, merged
- **Scope**: Replaced 3 incompatible control patterns (ControlMsg/ControlMsgRsp, cache.FlushReq/FlushRsp/RestartReq/RestartRsp, tlb.FlushReq/FlushRsp/RestartReq/RestartRsp) with single ControlReq/ControlRsp + ControlCommand enum.

### M58: DRAM improvements — predefined specs + open-page + FR-FCFS (#484) ✅
- **Status**: COMPLETE (cycle 409-413, ~4 cycles)
- **PR**: Merged to main (tara/dram-tests)
- **Scope**:
  1. Added predefined Spec structs for DDR4, DDR5, HBM2, HBM3, GDDR6 with timing parameters
  2. Added `PagePolicy` field to Spec (ClosePage/OpenPage), implemented open-page command creation
  3. Implemented FR-FCFS scheduling (row-buffer hit priority, then FCFS)
  4. Added read/write queue separation with configurable drain watermarks
  5. Added DDR5, HBM3, LPDDR5, HBM3E protocol enums
  6. 63 tests passing (46 new)
- **Human constraint**: Used spec structs, NOT config files

### M59: Quality cleanup — serialization bug, spec validation, repo hygiene ✅
- **Status**: COMPLETE (cycle 415-418, ~4 cycles)
- **PR**: #92, merged
- **Scope**: All 7 issues fixed — flush pointer flattened, DRAM timing moved to middleware, binaries gitignored, duplicate test removed, legacyMapper removed, WritePolicy replaced with string+switch, SameRank bug fixed

### M60: Fix CI lint + DRAM improvements phase 2 — validation + refresh + tFAW (#484) ✅
- **Status**: COMPLETE (cycle 420-428, ~8 cycles)
- **PR**: #93, merged
- **Scope**:
  1. Fixed all 6 CI lint errors (trailing newline, unused func, cognitive complexity, range-over-int, user-defined max)
  2. Analytical validation tests with exact cycle-count verification (309 lines)
  3. Periodic refresh scheduling and tFAW constraint enforcement (242 lines of tests)
  4. Basic DRAM statistics — bandwidth, latency tracking, row-buffer hit rate (42+134 lines)
  5. Acceptance test build-order fix for DRAM
- **Final**: 84 DRAM tests passing, all CI green on main

### M61: Integer ID Migration ✅
- **Status**: COMPLETE (cycle 432-436, ~5 cycles)
- **PR**: #94, merged
- **Scope**: Migrated ALL entity IDs from string to uint64 across all 7 subsystems in one pass: core types (MsgMeta, EventBase), ID generator, tracing (Task, Milestone), NOC, memory subsystem, Daisen, examples. 66 files changed.
- **Budget**: 8 cycles (used ~5)

### M62: DRAM Cross-Validation against DRAMSim3/Ramulator2 ✅
- **Status**: COMPLETE (cycle 438-442, ~4 cycles)
- **PR**: #95, merged
- **Scope**: Added `timing_crossvalidation_test.go` with 4-tier validation: (1) 66 timing formula cross-validation tests across DDR4/DDR5/HBM2, (2) 4 single-request latency tests, (3) 4 multi-request behavioral tests including tFAW, (4) 3 bandwidth sanity checks. Pure Go, Ginkgo/Gomega framework. 158 total DRAM tests (84 existing + 74 new).
- **Budget**: 6 cycles (used ~4)

### M63: Documentation overhaul ✅
- **Status**: COMPLETE (cycle 445-472, PR #96 merged)
- **Scope**: component_guide.md updated, migration.md (876 lines), docs.md (874 lines), 20 README.md files
- **Human issues addressed**: #587 (migration guide), #588 (comprehensive docs)

### Human Requests Status (Cycle 474)

- **Hook/tracing merge (#595)**: Research COMPLETE. Two independent researchers (Iris, Diana) both recommend: keep separate, clean up dead code. Recommendation posted to human. Awaiting response — NO implementation until authorized.
- **Merge AkitaRTM/Daisen (#482)** — Full merge **AUTHORIZED** (green light on #586). One Go server, one React frontend, one SQLite DB.
- **React rewrite (#488)** — **AUTHORIZED** as part of Daisen merge effort.

### Daisen/AkitaRTM Merge — Implementation Plan

Based on Mara's detailed analysis (issue #586, ~500 lines). Three phases:

**M64: Backend Merge — Combine Go servers** ✅ COMPLETE (~6 budgeted, ~5 used)
- Converted `v5/daisen` from `package main` to library package
- Moved CLI entry point to `v5/daisen/cmd/main.go`
- Merged AkitaRTM (monitoring) API endpoints into unified Daisen server
- Fixed milestone table naming mismatch
- Replaced gorilla/mux with stdlib net/http
- Added live mode with WAL-mode SQLite reader
- Updated `simulation/builder.go` to use unified Daisen server
- Live mode: monitoring + trace endpoints active; Replay mode: trace endpoints active, monitoring returns 503
- Merged to main (commit a326110)

**M65: React Frontend — Scaffold + Trace Visualization** (~8 cycles) — NEXT
- Scaffold React app with Vite + TypeScript in `v5/daisen/static/`
- Replace the old vanilla TS frontend
- Implement mode-aware navigation (live/replay)
- Migrate trace visualization: task chart (D3-based Gantt), task list, dashboard
- Migrate component view with analytics
- Update `go:embed` to serve the new React build

**M66: React Frontend — Live Monitoring + Chatbot** (~6 cycles)
- Migrate engine control panel (pause/continue/run/tick)
- Migrate progress bars, resource monitor, hang detector
- Migrate AI chatbot panel
- Unify live + replay navigation

**M67: Cleanup + Polish** (~4 cycles)
- Remove deprecated `v5/monitoring/` package entirely
- Remove old vanilla TS frontend code
- Live trace streaming (view traces while simulation runs)
- Final testing and polish

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
