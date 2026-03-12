# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware, make State canonical (no runtime copies), split monolithic middlewares into multiple stages.

## Current State (after M22, Cycle 190)

### What's Done

| Category | Status | Details |
|----------|--------|---------|
| modeling package | ✅ | `modeling.Component[Spec, State]` with A-B state (current/next, deep-copy, swap) |
| Messages as concrete types | ✅ | All messages are plain structs with `sim.MsgMeta` |
| Save/Load | ✅ | `simulation.Save/Load` works, acceptance test passes |
| 16 components ported | ✅ | All use `modeling.Component[Spec, State]` |
| MSHR/Directory as State + free functions | ✅ | Shared ops in `mem/cache/`, indices instead of pointers |
| Pipeline/Buffer as State (caches + switch) | ✅ | `queueing.Pipeline/Buffer` eliminated in caches/switch, adapters.go pattern |
| Dependencies inlined (DRAM) | ✅ | All internal packages eliminated, logic embedded |
| Dependencies inlined (caches) | ✅ | legacyMapper resolved at Build time, routing via Spec |
| A-B pattern correct | ✅ | All components use GetState() for reads, GetNextState() for writes |
| Comp wrapper elimination | ✅ | addresstranslator, datamover, MMU, GMMU, DRAM — Comp removed |
| CI passing | ✅ | Build, vet, tests all pass (PR #48 merged) |

### Per-Component Status

| Component | Comp Wrapper? | A-B Correct? | Multi-MW? | Notes |
|-----------|:---:|:---:|:---:|---|
| idealmemcontroller | thin (StorageOwner) | ✅ | ✅ 2 MW | Reference implementation |
| writearound cache | none | ✅ | ❌ 1 MW | — |
| writeevict cache | none | ✅ | ❌ 1 MW | — |
| writethrough cache | none | ✅ | ❌ 1 MW | — |
| writeback cache | none | ✅ | ❌ 1 MW | — |
| TLB | thin | ✅ | ✅ 2 MW | — |
| mmuCache | thin | ✅ | ✅ 2 MW | — |
| DRAM | none | ✅ | ❌ 1 MW | Comp eliminated, A-B fixed |
| addresstranslator | none | ✅ | ❌ 1 MW | Comp eliminated |
| datamover | none | ✅ | ❌ 1 MW | Comp eliminated |
| simplebankedmemory | thin (StorageOwner) | ✅ | ❌ 1 MW | — |
| MMU | none | ✅ | ❌ 1 MW | Comp eliminated |
| GMMU | none | ✅ | ❌ 1 MW | GMMU struct eliminated |
| endpoint | thin (API) | ✅ | ❌ 1 MW | NetworkPort/DefaultSwitchDst via methods |
| switch | thin (API) | ✅ | ❌ 1 MW | GetRoutingTable via method |
| tickingping | none | ✅ | ❌ 1 MW | Example component |

### Summary of Remaining Gaps

1. **13/16 components have exactly 1 middleware** — need splitting into multiple stages
2. **component_guide.md** needs update for final multi-MW architecture
3. **VictimFinder interface** still exists (unused in production but not removed)
4. **queueing.Buffer adapters** still used by switch for arbitration compatibility

---

## Phase 2: Multi-Middleware Split + Final Cleanup

### ✅ M21: Cache Components — Eliminate Runtime Copies + Inline Dependencies (DONE)
- Budget: 8 | Used: ~6 | PR: #46

### ✅ M21.5: Fix CI Lint Failures on Main (DONE)
- Budget: 2 | Used: 1 | PR: #47

### ✅ M22: Fix A-B Pattern + Eliminate Comp Wrappers (All Remaining Components) (DONE)
- Budget: 6 | Used: ~3 | PR: #48

### M23: Multi-Middleware Split — Endpoint, DRAM, Switch, SimpleBankedMemory, TickingPing

**Goal:** Split 5 components with the clearest stage boundaries into multiple middlewares.

**Target components and proposed splits:**
1. **endpoint** → 2 MW: incoming (network→local), outgoing (local→network). Zero shared state — trivial split.
2. **DRAM** → 3 MW: parseTop, bankTick, respond. Stages already free functions.
3. **tickingping** → 2 MW: receive+process, send. Example component.
4. **simplebankedmemory** → 2 MW: dispatch, tick+finalize.
5. **switch** → 2 MW: receive+pipeline, route+forward.

**Each split must:**
- Maintain correct A-B state semantics (read current, write next)
- Not break save/load (State struct unchanged)
- Pass existing tests

**Budget**: 6 cycles

### M24: Multi-Middleware Split — AddressTranslator, DataMover, MMU, GMMU

**Goal:** Split the remaining non-cache single-middleware components.

**Proposed splits:**
- **addresstranslator** → 2 MW: ctrl+translate, respond+pipeline
- **datamover** → 2 MW: ctrl+parse, data transfer
- **MMU** → 2 MW: translation engine, migration engine (MigrationQueue as natural boundary)
- **GMMU** → 2 MW: parse+walk, fetchFromBottom+respond

**Budget**: 6 cycles

### M25: Multi-Middleware Split — Cache Components

**Goal:** Split the 4 cache components into multiple middlewares.

**Proposed middleware boundaries:**
- **writearound/writeevict/writethrough** → 3 MW: FrontEnd (topParser/coalescer), Core (directory+bank), BackEnd (bottomParser+respond+control)
- **writeback** → 3 MW: FrontEnd (topParser), Core (directory+bank), BackEnd (writeBuffer+mshr+flusher)

**Key considerations:**
- Under current MiddlewareHolder.Tick(), all MWs execute sequentially within one Tick — zero added latency (per Iris's analysis)
- Each stage reads from `current`, writes to `next` — stages don't see each other's writes
- No State struct changes needed — inter-stage buffers already exist as named fields
- Start with one cache variant (writearound), replicate pattern to others

**Budget**: 8 cycles
**Risk**: Medium (reduced from high — Iris's analysis showed no latency impact and no state changes needed).

### M26: Final Cleanup + Documentation

**Goal:** Final pass — consistency, documentation, edge cases.

1. **Update component_guide.md** to reflect the final multi-middleware architecture with A-B state
2. **Review directconnection** — determine if it should use modeling.Component or stay as infrastructure
3. **Review examples/ping** — update if needed
4. **Ensure all components** follow the identical pattern consistently
5. **Full test suite pass** + acceptance tests
6. **Clean up any remaining thin Comp wrappers**
7. **Remove unused VictimFinder interface**
8. **Final code review pass**

**Budget**: 4 cycles

---

## Phase 2 Summary

| Milestone | Scope | Budget | Status |
|-----------|-------|--------|--------|
| M21 | Cache cleanup | 8 | ✅ Done (~6 used) |
| M21.5 | Fix CI lint failures | 2 | ✅ Done (1 used) |
| M22 | Fix A-B + eliminate Comp | 6 | ✅ Done (~3 used) |
| M23 | Multi-MW split — endpoint, DRAM, switch, simplebankedmemory, tickingping | 6 | ⬅️ NEXT |
| M24 | Multi-MW split — addresstranslator, datamover, MMU, GMMU | 6 | Pending |
| M25 | Multi-MW split — cache components | 8 | Pending |
| M26 | Final cleanup + docs | 4 | Pending |
| **Total Phase 2** | | **40** | |

---

## ✅ Completed Milestones (Phase 1)

| Milestone | Budget | Used | Description |
|-----------|--------|------|-------------|
| M1 | 6 | 5 | Create `modeling` package |
| M2 | 6 | 4 | Refactor idealmemcontroller |
| M3 | 8 | 6 | Save/Load with acceptance test |
| M4 | 3 | 2 | Fix CI lint failures |
| M5 | 8 | 6 | Messages as plain structs |
| M6 | 16 | 8 | Port all first-party components |
| M7 | 30 | 16 | Move mutable data into State |
| M8 | 24 | 18 | Msg-as-Interface redesign |
| M9 | 4 | 2 | Component creation guide |
| M10 | 2 | 3 | CI fix + Dependabot |
| M11 | 2 | 0 | Architecture design finalized |
| M12 | 5 | 3 | A-B state + Comp elimination on idealmemcontroller |
| M13 | 5 | 3 | TLB — Comp elimination + A-B state |
| M14 | 6 | 3 | Simple Components Batch (4 components) |
| M15 | 5 | 3 | GMMU + Switch + Endpoint — Comp elimination + A-B state |
| M16 | 8 | 4 | Write{around,evict,through} caches + tickingping |
| M17 | 6 | 3 | Writeback cache — Full transformation |
| M18 | 8 | 3 | DRAM memory controller — Full transformation |
| M19 | 4 | 2 | MMU — Full transformation (thin Comp, canonical State) |
| M20 | 4 | 2 | Switch — State canonical, eliminate queueing objects |
| M21 | 8 | ~6 | Cache components — eliminate runtime copies, inline deps, A-B state |
| M21.5 | 2 | 1 | Fix CI lint failures (25 errors) |
| M22 | 6 | ~3 | Fix A-B pattern + eliminate Comp wrappers |

**Phase 1 totals**: Budget: 160, Used: ~100 (37% under budget)

## Lessons Learned

- CI can get stuck in "queued" state — don't waste cycles waiting for it
- Architecture discussions should be fully resolved before implementation
- Multi-worker mechanical changes work well with clear patterns
- Breaking milestones to 2-6 cycle budgets is optimal
- Human feedback drives direction — stay responsive
- idealmemcontroller is the reference implementation — follow its patterns
- The snapshot/restore conversion layer disappears when State is canonical
- A-B state deep copy via JSON round-trip is acceptable for small States
- Components with external services (Storage, PageTable, RoutingTable) keep those as middleware fields
- The 3 simpler caches are nearly identical — transform one, replicate twice
- Budget estimates are improving: most milestones finish well under budget
- Shared free functions (directory_ops.go, mshr_ops.go) are reusable across cache types
- **Always run lint before merging**: M21 introduced 25 lint errors. CI-fix milestones waste cycles.
- **A-B pattern was not enforced during earlier milestones** — many components were "ported" but still use GetNextState() for reads. Need explicit audit step in future milestones.
- **Multi-middleware split is the next major architectural change** — needs careful planning per component
- **M22 finished well under budget (3 of 6 cycles)** — continue aggressive budgeting
