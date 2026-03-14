# Roadmap

## Project Goal

Evolve Akita V5 toward a clean, high-performance simulation framework with broad DRAM support, unified protocols, modern visualization, and clean architecture.

## Current State (Cycle 376)

### Previous Phase: ✅ COMPLETE (M1-M50)

All 50 milestones from the original component model refactoring are complete. All 16 success criteria met. CI green on main.

### Current Phase: Research & Discussion — ✅ RESEARCH COMPLETE

All 13 human topics have been researched. Findings summarized below. **Awaiting human authorization before any implementation.**

---

## Phase 2: Research Findings & Recommendations

### Topic 1: Move directconnection to noc? (#477)
**Recommendation: YES** — `noc/networkconnector` already type-asserts on `*directconnection.Comp`. Moving it is organizational improvement. 10 files need import updates. No circular dependency risk.

### Topic 2: Component-engine decoupling (#478)
**Recommendation: YES** — `EventScheduler` interface already exists. Components only use `Schedule()` and `CurrentTime()`, never `Run()`/`Pause()`. Change is mechanical: replace `Engine` fields with `EventScheduler`.

### Topic 3: Event serialization (#479)
**Recommendation: Replace `Handler()` with `HandlerID() string` + `HandlerRegistry`** — Events store handler name instead of pointer. Engine looks up handler at dispatch. Enables arbitrary checkpoint/restore without quiescence.

### Topic 4: Integer time representation (#480)
**Recommendation: YES, uint64 picoseconds** — Every major simulator uses integer time. `Freq` methods currently use `math.Round` hacks as precision workarounds. 1 GHz = 1000 ps (exact). Migration scope: ~21 core files, compiler catches all breakage.

### Topic 5: Merge concrete simulators into Akita? (#481)
**Recommendation: NO** — Keep separate repos. Simulators are domain-independent (AMD GPU, Apple CPU, NVIDIA, Tenstorrent). MGPUSim alone is 72K LoC vs Akita's 52K. Use `go.work` for development coordination.

### Topic 6: Merge AkitaRTM and Daisen? (#482)
**Recommendation: PARTIAL MERGE** — Full merge not recommended (different runtime models: embedded library vs standalone binary). Recommended: unified frontend with live/replay modes, shared Go infrastructure, seamless handoff from monitoring to visualization.

### Topic 7: Double buffering residue (#483)
**Recommendation: CLEAN UP** — 17 non-test files still use `GetState()`/`GetNextState()` split pattern from old double-buffering. Code is correct but misleading. Worst offenders: datamover (8 functions), mmu/translationmw.go (counter decrement pattern), noc/switching (4 files). Human specifically flagged tlbmiddleware.go line 28 (actually clean; the residue is in ctrlmiddleware.go same package).

### Topic 8: DRAM controller improvements (#484)
**Recommendation: Hybrid approach** — Build timing/scheduling natively in Go (preserves save/load). Parse DRAMSim3 .ini / Ramulator2 YAML for parameters. Critical gaps: no open-page policy, no FR-FCFS scheduling, no DDR5/HBM3, no power modeling. 4-phase roadmap proposed.

### Topic 9: Remove idealmemcontroller? (#485)
**Recommendation: NO, keep it** — simplebankedmemory lacks control port and unlimited concurrency. idealmemcontroller is only ~350 LOC and used by 6 acceptance tests. Low maintenance burden.

### Topic 10: /mem/mem → /mem flattening (#486)
**Recommendation: YES** — Parent `mem/` has zero Go source files. `mem/mem` stutters. 43 files need mechanical import change. Standard Go convention supports files alongside subdirectories.

### Topic 11: Unified memory control protocol (#487)
**Recommendation: YES** — 3 incompatible patterns exist (bitfield ControlMsg, cache FlushReq/RestartReq, TLB-specific). Replace with single `ControlReq`/`ControlRsp` using enum commands. DRAM and simplebankedmemory currently have NO control interface. Affects 10 internal packages.

### Topic 12: Rewrite Daisen/AkitaRTM with React (#488)
**Recommendation: YES, React** — Current frontends use vanilla TS+D3 with manual DOM manipulation (~12,700 LOC). React brings component reusability, ecosystem, maintainability. **Critical: merge frontends first (Topic 6), then rewrite.** Vue is reasonable alternative.

### Topic 13: No implementation without authorization (#489)
**Status: Acknowledged.** All work has been research-only.

---

## Proposed Implementation Order (Pending Human Authorization)

Based on dependency analysis and impact:

### Tier 1: Foundational changes (should be done first)
1. **Double buffering cleanup** (#483) — Small, low-risk, removes confusion
2. **Component-engine decoupling** (#478) — Enables event serialization
3. **Integer time** (#480) — Fundamental type change, should be done before other refactors
4. **Event serialization** (#479) — Depends on #478 and #480
5. **/mem/mem flattening** (#486) — Mechanical, breaks many import paths (do early)

### Tier 2: Architecture improvements
6. **Move directconnection to noc** (#477) — Small organizational change
7. **Unified control protocol** (#487) — Depends on understanding from Tier 1 work
8. **DRAM controller improvements** (#484) — Large, can be phased

### Tier 3: Frontend/tooling (independent track)
9. **Partial merge AkitaRTM/Daisen** (#482) — Can proceed independently
10. **React rewrite** (#488) — Depends on #482

### Not recommended for implementation:
- Merging concrete simulators (#481) — Keep separate repos

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
