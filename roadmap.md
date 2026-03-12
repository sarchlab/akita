# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware, make State canonical (no runtime copies), split monolithic middlewares into multiple stages.

## Current State (after M21.5, Cycle 180)

### What's Done

| Category | Status | Details |
|----------|--------|---------|
| modeling package | ✅ | `modeling.Component[Spec, State]` with A-B state (current/next, deep-copy, swap) |
| Messages as concrete types | ✅ | All messages are plain structs with `sim.MsgMeta` |
| Save/Load | ✅ | `simulation.Save/Load` works, acceptance test passes |
| 16 components ported | ✅ | All use `modeling.Component[Spec, State]` |
| MSHR/Directory as State + free functions | ✅ | Shared ops in `mem/cache/`, indices instead of pointers |
| Pipeline/Buffer as State (caches + switch) | ✅ | `queueing.Pipeline/Buffer` eliminated, adapters.go pattern |
| Dependencies inlined (DRAM) | ✅ | All internal packages eliminated, logic embedded |
| Dependencies inlined (caches) | ✅ | legacyMapper resolved at Build time, routing via Spec |
| CI passing | ✅ | All 25 lint errors from M21 fixed (PR #47) |

### What's NOT Done — Per-Component Audit

| Component | Comp Wrapper? | A-B Correct? | Multi-MW? | Other Issues |
|-----------|:---:|:---:|:---:|---|
| idealmemcontroller | thin (StorageOwner) | ✅ | ✅ 2 MW | Reference implementation |
| writearound cache | none | ✅ | ❌ 1 MW | — |
| writeevict cache | none | ✅ | ❌ 1 MW | — |
| writethrough cache | none | ✅ | ❌ 1 MW | — |
| writeback cache | none | ✅ | ❌ 1 MW | — |
| TLB | thin | ✅ | ✅ 2 MW | — |
| mmuCache | thin | ✅ | ✅ 2 MW | — |
| DRAM | ❌ has Comp | ❌ GetNextState for reads | ❌ 1 MW | Comp has topPort, storage |
| addresstranslator | ❌ has Comp | needs audit | ❌ 1 MW | Comp is empty wrapper |
| datamover | ❌ has Comp | ❌ GetNextState for reads | ❌ 1 MW | Comp is empty wrapper |
| simplebankedmemory | ❌ has Comp | ❌ GetNextState for reads | ❌ 1 MW | Comp has storage |
| MMU | ❌ has Comp | ❌ GetNextState for reads | ❌ 1 MW | Comp is empty wrapper |
| GMMU | ❌ has GMMU struct | needs audit | ❌ 1 MW | GMMU wraps Component |
| endpoint | ❌ has Comp | ❌ GetNextState for reads | ❌ 1 MW | Comp has NetworkPort, etc |
| switch | ❌ has Comp | ❌ GetNextState for reads | ❌ 1 MW | Comp has mw field |
| tickingping | thin | needs audit | ❌ 1 MW | Example component |

### Summary of Remaining Gaps

1. **A-B pattern incorrect in ~10 components** — they use `GetNextState()` for both reads and writes instead of `GetState()` for reads
2. **Comp wrapper structs exist in ~10 components** — need elimination (move storage/external refs to middleware, remove wrapper)
3. **13/16 components have exactly 1 middleware** — need splitting into multiple stages
4. **component_guide.md** needs update for final architecture

---

## Phase 2: Fix A-B Pattern + Eliminate Comp Wrappers + Multi-MW Split

### ✅ M21: Cache Components — Eliminate Runtime Copies + Inline Dependencies (DONE)
- Budget: 8 | Used: ~6 | PR: #46

### ✅ M21.5: Fix CI Lint Failures on Main (DONE)
- Budget: 2 | Used: 1 | PR: #47

### M22: Fix A-B Pattern + Eliminate Comp Wrappers (All Remaining Components)

**Goal:** Two changes across ALL remaining components:
1. Fix A-B state pattern: change `GetNextState()` reads → `GetState()` reads everywhere
2. Eliminate Comp wrapper structs: move `*mem.Storage` / external refs to middleware fields, remove Comp type

**Components to fix (grouped by complexity):**

**Simple (Comp is empty wrapper, just A-B fix):**
- addresstranslator — remove Comp, fix A-B
- datamover — remove Comp, fix A-B
- MMU — remove Comp, fix A-B
- GMMU — remove GMMU struct, fix A-B

**Medium (Comp holds storage or ports, need to move to middleware):**
- DRAM — move topPort + storage to middleware, remove Comp, fix A-B
- simplebankedmemory — move storage to middleware, remove Comp, fix A-B  
- tickingping — fix A-B, slim Comp

**Complex (Comp holds runtime refs + other state):**
- endpoint — move NetworkPort/DefaultSwitchDst to middleware, remove Comp, fix A-B
- switch — move mw ref to middleware, remove Comp, fix A-B

**Budget**: 6 cycles
**Risk**: Medium. Mechanical changes, but need to be careful about external callers that use `Comp` type in their APIs (builders, acceptance tests, etc).

### M23: Multi-Middleware Split — Simple Components

**Goal:** Split the simpler single-middleware components into multiple middlewares following natural stage boundaries.

**Target components and proposed splits:**
1. **addresstranslator** → 2 MW: parse incoming, send response
2. **datamover** → 2 MW: control, data transfer
3. **simplebankedmemory** → 2 MW: control, memory operations
4. **tickingping** → 2 MW: receive, send
5. **GMMU** → 2 MW: parse, walk+respond

**Each split must:**
- Maintain correct A-B state semantics (read current, write next)
- Not break save/load (State struct unchanged)
- Pass existing tests

**Budget**: 6 cycles

### M24: Multi-Middleware Split — DRAM + Network Components

**Goal:** Split DRAM, endpoint, and switch into multiple middlewares.

**Proposed splits:**
- **DRAM** → 3 MW: parseTop (receive requests, create sub-transactions), bankTick (process commands, update timing), respond (send completed transactions)
- **endpoint** → 2 MW: incoming (network→local), outgoing (local→network)
- **switch** → 2-3 MW: receive, route/arbitrate, forward

**Budget**: 6 cycles

### M25: Multi-Middleware Split — Cache Components

**Goal:** Split the 4 cache components into multiple middlewares.

**Proposed middleware boundaries:**
- **writearound/writeevict/writethrough**: topParser → directory → bank → bottomParser → respond (5 stages)
- **writeback**: topParser → directory → bank → writeBuffer → mshr → flusher (6 stages)

**Key considerations:**
- +1 cycle latency per middleware boundary
- Each stage reads from `current`, writes to `next` — stages don't see each other's writes
- Currently, all stages run within a single middleware's Tick() — the split makes them independent middlewares

**Budget**: 8 cycles
**Risk**: High. Cache split is the most complex change. The writeback cache has the most stages and complex inter-stage data flow.

### M26: Final Cleanup + Documentation

**Goal:** Final pass — consistency, documentation, edge cases.

1. **Update component_guide.md** to reflect the final multi-middleware architecture with A-B state
2. **Review directconnection** — determine if it should use modeling.Component or stay as infrastructure
3. **Review examples/ping** — update if needed
4. **Ensure all components** follow the identical pattern consistently
5. **Full test suite pass** + acceptance tests
6. **Clean up any remaining thin Comp wrappers**
7. **Final code review pass**

**Budget**: 4 cycles

---

## Phase 2 Summary

| Milestone | Scope | Budget | Status |
|-----------|-------|--------|--------|
| M21 | Cache cleanup | 8 | ✅ Done (~6 used) |
| M21.5 | Fix CI lint failures | 2 | ✅ Done (1 used) |
| M22 | Fix A-B pattern + eliminate Comp wrappers | 6 | ⬅️ NEXT |
| M23 | Multi-MW split — simple components | 6 | Pending |
| M24 | Multi-MW split — DRAM + network | 6 | Pending |
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
