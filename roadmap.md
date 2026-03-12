# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware, make State canonical (no runtime copies), split monolithic middlewares into multiple stages.

## Current State (after M23, Cycle 199)

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
| CI passing | ✅ | Build, vet, tests all pass (PR #49 merged) |
| Multi-MW split (batch 1) | ✅ | endpoint(2), DRAM(3), switch(2), simplebankedmemory(2), tickingping(2) |

### Per-Component Status

| Component | Comp Wrapper? | A-B Correct? | Multi-MW? | MW Count | Notes |
|-----------|:---:|:---:|:---:|:---:|---|
| idealmemcontroller | thin (StorageOwner) | ✅ | ✅ | 2 | Reference implementation |
| writearound cache | none | ✅ | ❌ | 1 | Need split |
| writeevict cache | none | ✅ | ❌ | 1 | Need split |
| writethrough cache | none | ✅ | ❌ | 1 | Need split |
| writeback cache | none | ✅ | ❌ | 1 | Need split |
| TLB | thin | ✅ | ✅ | 2 | Done |
| mmuCache | thin | ✅ | ✅ | 2 | Done |
| DRAM | none | ✅ | ✅ | 3 | Done (M23) |
| addresstranslator | none | ✅ | ❌ | 1 | Need split |
| datamover | none | ✅ | ❌ | 1 | Need split |
| simplebankedmemory | thin (StorageOwner) | ✅ | ✅ | 2 | Done (M23) |
| MMU | none | ✅ | ❌ | 1 | Need split |
| GMMU | none | ✅ | ❌ | 1 | Need split |
| endpoint | thin (API) | ✅ | ✅ | 2 | Done (M23) |
| switch | thin (API) | ✅ | ✅ | 2 | Done (M23) |
| tickingping | none | ✅ | ✅ | 2 | Done (M23) |

### Summary of Remaining Gaps

1. **8/16 components still have 1 middleware** — addresstranslator, datamover, MMU, GMMU, writeback, writearound, writeevict, writethrough
2. **component_guide.md** needs update for final multi-MW architecture with A-B state
3. **VictimFinder interface** still exists (unused in production but not removed)
4. **queueing.Buffer adapters** still used by switch for arbitration compatibility
5. **examples/ping** still uses old event-driven model (not modeling.Component)

---

## Phase 2: Multi-Middleware Split + Final Cleanup

### ✅ M21: Cache Components — Eliminate Runtime Copies + Inline Dependencies (DONE)
- Budget: 8 | Used: ~6 | PR: #46

### ✅ M21.5: Fix CI Lint Failures on Main (DONE)
- Budget: 2 | Used: 1 | PR: #47

### ✅ M22: Fix A-B Pattern + Eliminate Comp Wrappers (All Remaining Components) (DONE)
- Budget: 6 | Used: ~3 | PR: #48

### ✅ M23: Multi-Middleware Split — Endpoint, DRAM, Switch, SimpleBankedMemory, TickingPing (DONE)
- Budget: 6 | Used: ~5 | PR: #49
- Split 5 components into multiple middlewares
- endpoint → 2 MW, DRAM → 3 MW, switch → 2 MW, simplebankedmemory → 2 MW, tickingping → 2 MW

### M24: Multi-Middleware Split — AddressTranslator, DataMover, MMU, GMMU ⬅️ NEXT

**Goal:** Split the remaining 4 non-cache single-middleware components into multiple middlewares.

**Proposed splits:**
- **addresstranslator** (~660 LOC) → 2 MW: parse+translate (accept requests, start translation), respond+pipeline (count down, send responses)
- **datamover** (~600 LOC) → 2 MW: ctrl+parse (accept data move requests), data transfer (perform reads/writes, respond)
- **MMU** (~540 LOC) → 2 MW: translation engine (page table walks), migration engine (MigrationQueue as natural boundary)
- **GMMU** (~380 LOC) → 2 MW: parse+walk (accept requests, walk page table), fetchFromBottom+respond (handle memory responses, send replies)

**Each split must:**
- Maintain correct A-B state semantics (read current, write next)
- Not break save/load (State struct unchanged)
- Pass existing tests
- Pass lint (zero new lint errors vs main)

**Budget**: 6 cycles

### M25: Multi-Middleware Split — Cache Components

**Goal:** Split the 4 cache components into multiple middlewares.

**Proposed middleware boundaries:**
- **writearound/writeevict/writethrough** (~2,700-2,800 LOC each) → 3 MW: FrontEnd (topParser/coalescer), Core (directory+bank), BackEnd (bottomParser+respond+control)
- **writeback** (~3,700 LOC) → 3 MW: FrontEnd (topParser), Core (directory+bank), BackEnd (writeBuffer+mshr+flusher)

**Key considerations:**
- Under current MiddlewareHolder.Tick(), all MWs execute sequentially within one Tick — zero added latency
- Each stage reads from `current`, writes to `next` — stages don't see each other's writes
- No State struct changes needed — inter-stage buffers already exist as named fields
- Start with one cache variant (writearound), replicate pattern to others
- The 3 simpler caches (writearound, writeevict, writethrough) share near-identical code structure

**Budget**: 8 cycles
**Risk**: Medium — caches are the largest and most complex components.

### M26: Final Cleanup + Documentation

**Goal:** Final pass — consistency, documentation, edge cases.

1. **Update component_guide.md** to reflect the final multi-middleware architecture with A-B state
2. **Review directconnection** — determine if it should use modeling.Component or stay as infrastructure
3. **Review examples/ping** — update to modeling.Component or document as legacy
4. **Ensure all components** follow the identical pattern consistently
5. **Full test suite pass** + acceptance tests
6. **Clean up any remaining thin Comp wrappers** where possible
7. **Remove unused VictimFinder interface** if no longer referenced
8. **Remove queueing.Buffer adapters** if possible
9. **Final code review pass**

**Budget**: 4 cycles

---

## Phase 2 Summary

| Milestone | Scope | Budget | Used | Status |
|-----------|-------|--------|------|--------|
| M21 | Cache cleanup | 8 | ~6 | ✅ Done |
| M21.5 | Fix CI lint failures | 2 | 1 | ✅ Done |
| M22 | Fix A-B + eliminate Comp | 6 | ~3 | ✅ Done |
| M23 | Multi-MW split (batch 1: 5 components) | 6 | ~5 | ✅ Done |
| M24 | Multi-MW split (batch 2: 4 non-cache) | 6 | — | ⬅️ NEXT |
| M25 | Multi-MW split (batch 3: 4 caches) | 8 | — | Pending |
| M26 | Final cleanup + docs | 4 | — | Pending |
| **Total Phase 2** | | **40** | **~15** | |

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
| M21 | 8 | ~6 | Cache components — eliminate runtime copies, inline deps |
| M21.5 | 2 | 1 | Fix CI lint failures (25 errors) |
| M22 | 6 | ~3 | Fix A-B pattern + eliminate Comp wrappers |
| M23 | 6 | ~5 | Multi-MW split — 5 components |

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
- **Multi-middleware split is mechanical** — M23 completed efficiently with one worker per component. Continue this pattern.
- **M23 needed a fix round** — flit metadata loss in endpoint/switch caught by verification. Always verify carefully.
- **Assign lint-checking to workers explicitly** — don't assume it happens automatically.
