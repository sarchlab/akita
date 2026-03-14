# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 350)

### Project Status: IN PROGRESS — Investigating M45.2 + M46 design

**M45.1 complete:** Analysis package removed, WriteThroughCache coalescer removed and transaction model flattened. PR #74 merged.

**Remaining human issues:**
- #439: Standardize component file paradigm (state.go, spec.go, one file per MW)
- #440: sim.Component vs modeling.Component consolidation
- #408: Repo-wide simplification (find wrappers, indirections, residues)
- #389: Event-driven component support (high priority, complex)

**Investigation in progress (cycle 350):**
- Elena auditing all 14 component packages for file paradigm + simplification
- Iris designing event-driven component architecture (studying TrioSim)

### Recently Completed

#### M45.1: Remove analysis package + WriteThroughCache coalescer (DONE — Cycle 350)
- Budget: 8 | Used: ~4
- PR #74 merged
- analysis/ directory deleted, no analysis imports remain
- Coalescer removed from WriteThroughCache, transaction model flattened (-698 lines)
- Issues #432, #434 resolved

#### M44: Repo-wide cleanup — delete dead code and packages (DONE — Cycle ~342)
- Budget: 6 | Used: ~4
- PR #73 merged

#### M43: Consolidate stateutil into queueing (DONE — Cycle ~334)
- Budget: 8 | Used: ~6
- PR #72 merged

---

## Planned Milestones

### M45.2: Component file paradigm standardization (PLANNED — estimated 8 cycles)
- **Component file paradigm** (human issue #439): Standardize file organization:
  - `spec.go` — Spec struct and all sub-structs
  - `state.go` — State struct and all sub-structs
  - One file per middleware (named after the middleware)
  - `builder.go` — builder
  - `doc.go` — package documentation
- Rename/move files across all 14 component packages
- **sim.Component question** (human issue #440): Keep sim.Component (load-bearing for ports), keep modeling.Component in modeling package
- **Simplification** (human issue #408): Remove any remaining wrappers, Comp structs, residues found by Elena's audit

### M46: Event-driven component support (estimated 8-12 cycles)
- Design event-driven component pattern that:
  - Follows Spec+State model
  - Supports save/load (serializable events)
  - No tick overhead — real event scheduling
  - Coexists with tick-based components
- Implement in modeling package
- Port examples/ping to new pattern
- Address issue #389
- See TrioSim for real-world need

### M47: Minor performance optimizations (estimated 4-6 cycles)
- Buffer ring buffer pattern
- Tracing guards
- Switch/endpoint performance

### M48: Global state manager (deferred, estimated 3-5 cycles)
- Single-call save/load of entire simulation state

---

## Success Criteria Checklist

| # | Criterion | Status |
|---|-----------|--------|
| 1 | Simple, intuitive APIs | ✅ |
| 2 | All CI checks pass on main | ✅ |
| 3 | Component = Spec + State + Ports + MW + Hooks | ✅ |
| 4 | No Comp wrappers (except StorageOwner) | ✅ |
| 5 | No external dependency interfaces | ✅ |
| 6 | Single sim-level save/load | ✅ |
| 7 | Developers only implement MW Tick | ✅ |
| 8 | All runtime data in State | ✅ |
| 9 | No SaveState/LoadState conversion layers | ✅ |
| 10 | No restoreFromState/syncToState | ✅ |
| 11 | No runtime copies of State in MW | ✅ |
| 12 | Save/load acceptance test passes | ✅ |
| 13 | All components use modeling package | ✅ |
| 14 | Each component has multiple MWs | ✅ |
| 15 | component_guide.md reflects final arch | ✅ |
| 16 | Performance matches original | Under evaluation |

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
