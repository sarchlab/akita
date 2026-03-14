# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 351)

### Project Status: IN PROGRESS — Defining M45.2 (file paradigm standardization)

**M45.1 complete:** Analysis package removed, WriteThroughCache coalescer removed and transaction model flattened. PR #74 merged.

**Investigation complete (cycle 350):**
- Elena completed full audit of all 14 component packages → workspace/elena/note.md
- Iris completed event-driven component architecture design → workspace/iris/note.md (recommends Design B: State-Encoded Timers)

**Remaining human issues:**
- #439: Standardize component file paradigm (state.go, spec.go, one file per MW) → **M45.2**
- #440: sim.Component vs modeling.Component consolidation → deferred (sim.Component is load-bearing)
- #408: Repo-wide simplification → addressed via M45.2 file cleanup + Comp elimination
- #389: Event-driven component support → **M46** (Iris's Design B)

---

## Active/Planned Milestones

### M45.2: Component file paradigm standardization (ACTIVE — 10 cycles)
- Standardize all 14 component packages to: `spec.go`, `state.go`, one lowercase file per middleware, `builder.go`, `doc.go`
- Split monolithic files (endpoint 518 lines, addresstranslator 590+, simplebankedmemory 378, switches 291)
- Rename camelCase MW files to lowercase
- Eliminate switches Comp wrapper (zero extra fields)
- Fix typos (DateMovePort → DataMovePort)
- Verify mmuCache/mmucache directory situation
- Issues: #439, #408 (partial), #450

### M46: Event-driven component support (estimated 8-10 cycles)
- Implement `EventDrivenComponent[S, T]` in modeling package (Design B from Iris)
- Single event type (TimerFiredEvent), EventProcessor interface
- State-encoded timers for save/load compatibility
- ScheduleWakeAt/ScheduleWakeNow for dedup'd event scheduling
- Port examples/ping to new pattern as proof-of-concept
- Address issue #389

### M47: Minor performance optimizations (estimated 4-6 cycles)
- Buffer ring buffer pattern
- Tracing guards
- Switch/endpoint performance

### M48: Global state manager (deferred, estimated 3-5 cycles)
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
| **Total** | **45.1 milestones** | **~298** | **~195** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- In-place state update is simpler AND faster than deep copy
- Event-driven components represent a fundamental architectural challenge — different from tick-driven model
- Coalescer removal was less risky than expected — good analysis upfront paid off
- Always verify what's merged to main before defining next milestone
- Using investigator agents (Elena, Iris) to audit/design before coding milestones works well — invest in research upfront
- Large mechanical refactorings benefit from parallelizing across multiple workers
