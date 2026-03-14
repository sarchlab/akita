# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Single simulation-level save/load. No per-component custom code. No performance compromise. Developers focus only on middleware Tick logic.

## Current State (Cycle 337)

### Project Status: IN PROGRESS — M44 defined (repo cleanup)

**Performance resolved:** Diana's benchmarks confirm NOC is at v4 wall-clock parity (0.99x-1.2x) with 43-79x less peak memory after M42/M43. Issue #387 closed.

**Elena's audit complete:** Found 3 dead packages (~570 lines), 4 dead files (~78 lines), duplicated utilities, stale comments. All confirmed with zero imports.

**M44 defined:** Delete dead packages (arbitration, wiring, standalone), dead files, stale comments, and extract shared utilities (convertAddress, Flush/Restart msgs, LRU set ops, MSHR ops). See issue #431.

### Recently Completed

#### M43: Consolidate stateutil into queueing + pipeline migration (DONE — Cycle ~334)
- Budget: 8 | Used: ~6
- PR #72 merged
- stateutil package deleted entirely
- Buffer[T] and Pipeline[T] consolidated into queueing package
- Pop, PopTyped, Peek removed from Buffer
- Old queueing.Buffer interface and bufferImpl removed
- simplebankedmemory and TLB hand-coded pipelines replaced with queueing.Pipeline[T]
- Issue #414 closed

#### M42: Switch and endpoint simplification (DONE — Cycle ~326)
- Budget: 10 | Used: ~10
- PR #71 merged
- Switch 735→290 lines, endpoint 585→519 lines
- Human issues #402-#406 resolved

#### M41: Restore test sizes + fix CI duplication (DONE — Cycle 314)
- Budget: 2 | Used: 2
- PR #70 merged

#### M40: Rename simplecache, DefaultSpec, Freq in Spec (DONE — Cycle 309)
- Budget: 8 | Used: ~4
- PR #69 merged

### Human Directions — Status

#### Ultimate Goal (issue #342)
1. Single simulation-level save and load ✅
2. No per-component custom save/load functions ✅
3. Developers only implement middleware Tick functions ✅
4. No performance compromise — needs re-evaluation (#387)

#### Serializable Buffers/Pipelines (issue #343) — DONE

#### Global State Manager (issue #326) — DEFERRED

#### Default Spec / Rename / Freq (issue #384) — DONE in M40

#### Consolidate stateutil into queueing (issue #414) — DONE in M43

#### Event-Driven Components (issue #389) — NEEDS DESIGN
- Some components schedule events rather than ticking
- Human rejected tick-based workarounds — must be real event-driven
- Iris researched approaches; design needed

#### Performance Evaluation (issue #387) — NEEDS RE-EVALUATION
- Previous benchmark (pre-M42): NOC 3-12x slower, memory 1.1-2.2x slower
- M42 simplified switch/endpoint significantly
- M43 replaced hand-coded pipelines
- Need fresh evaluation after these changes

#### Repo-wide Simplification (issue #408) — PARTIALLY DONE
- ✅ Pipeline duplication (F2) — fixed by M43
- ✅ Flit serialization types (F4) — fixed by M42
- Remaining: F1 (convertAddress), F3 (Flush/Restart msgs), F5 (LRU set ops), F6 (MSHR ops)
- Also: search for double-buffering residues (human comment)

---

## Planned Milestones

### M44: Repo-wide cleanup — delete dead code and packages (estimated 6 cycles)
- Delete dead packages: arbitration/, wiring/ (+ 154 sqlite3 files), standalone/
- Delete dead files: domain.go, requestbuffer.go, transferevent.go, trafficcounter.go
- Delete dead types: LogHook interface, ConvertInternalToExternal panic method
- Fix stale comment in endpoint about JSON deep copy
- Extract shared convertAddress (F1)
- Unify Flush/Restart message types (F3)
- Extract shared LRU set ops (F5)
- Extract shared MSHR ops (F6)
- Issue #431

### M45: Event-driven component support (estimated 8-12 cycles)
- Design not timer-based (human rejected tick-based)
- Create modeling variant or alternative pattern
- Must support save/load of pending events
- See TrioSim for real-world need
- Issue #389

### M46: Minor performance optimizations (estimated 4-6 cycles)
- Buffer ring buffer pattern (replace sliding-window FIFO)
- Tracing guards (NumHooks check before string allocation)
- Switch sendOut flit heap escape fix
- Endpoint linear search → O(1) lookup

### M47: Global state manager (deferred, estimated 3-5 cycles)
- Single-call save/load of entire simulation state
- Depends on all components being fully standardized

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
| 16 | Performance matches original | Under evaluation (#387) |

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
| Phase 9 (M43) | Consolidate stateutil→queueing + pipeline migration | 8 | ~6 |
| **Total** | **43 milestones** | **~284** | **~187** |

---

## Lessons Learned

- Budget estimates improving: most milestones finish well under budget
- Human direction can pivot rapidly — stay responsive, don't over-plan
- Analysis/design phases pay off by preventing wrong implementations
- A-B deep copy was 100% wasted — always verify assumptions before building infrastructure
- In-place state update is simpler AND faster
- Cache adapter wrappers are complex boilerplate — generic types eliminated them
- Performance is non-negotiable — every change must be measured
- Transaction serialization (M38) was the last major piece — flattening pointer fields eliminated all conversion layers
- Documentation verification caught fabricated code — always verify code snippets against actual source files
- Event-driven components represent a new architectural challenge — different from tick-driven model
- Elena's audit missed acceptance_test.py changes — auditing only Go source files is insufficient
- Human's switch feedback (#402-#406) shows we over-engineered the serialization layer
- Performance research showed the copy-per-tick pattern is a fundamental bottleneck with large slices
- stateutil→queueing consolidation was straightforward — always consolidate duplicated packages early
