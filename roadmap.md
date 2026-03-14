# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 356)

### Project Status: IN PROGRESS — Defining M46 (event-driven component support)

**M45.2 complete and merged to main:** All 14 component packages standardized to spec.go/state.go/mwname.go paradigm. Monolithic files split, MW filenames lowercase, switches Comp wrapper eliminated, DateMovePort→DataMovePort typo fixed.

**Remaining human issues:**
- #389: Event-driven component support → **M46** (next)
- #440: sim.Component vs modeling.Component consolidation → deferred until after M46
- #408: Repo-wide simplification → ongoing, largely addressed by M45.x

---

## Active/Planned Milestones

### M46: Event-driven component support (NEXT — estimated 8 cycles)
- Implement `EventDrivenComponent[S, T]` in modeling package using Design B (State-Encoded Timers)
- Single event type (TimerFiredEvent), EventProcessor interface
- State-encoded timers for save/load compatibility
- ScheduleWakeAt/ScheduleWakeNow for dedup'd event scheduling
- Port examples/ping to new pattern as proof-of-concept
- Integrate with simulation save/load (WakeupResetter)
- File paradigm: eventdriven.go, eventdriven_builder.go in modeling/
- Address issue #389

### M47: sim.Component consolidation (estimated 4-6 cycles)
- Evaluate whether modeling.Component should move into sim package
- Simplify sim.Component interface if possible
- Address issue #440

### M48: Performance optimizations (estimated 4-6 cycles)
- Buffer ring buffer pattern
- Tracing guards
- Switch/endpoint performance

### M49: Global state manager (deferred, estimated 3-5 cycles)
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
| **Total** | **45.2 milestones** | **~308** | **~203** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- In-place state update is simpler AND faster than deep copy
- Event-driven components represent a fundamental architectural challenge — different from tick-driven model
- Using investigator agents (Elena, Iris) to audit/design before coding milestones works well
- Large mechanical refactorings benefit from parallelizing across multiple workers
- Iris's Design B (State-Encoded Timers) is recommended for event-driven components
- Always merge verified branches to main before moving on
