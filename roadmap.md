# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware, make State canonical (no runtime copies), split monolithic middlewares into multiple stages.

## Current State (after M25, Cycle 210)

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
| CI passing | ✅ | Build, vet, tests all pass (PR #51 merged) |
| Multi-MW split (batch 1) | ✅ | endpoint(2), DRAM(3), switch(2), simplebankedmemory(2), tickingping(2) |
| Multi-MW split (batch 2) | ✅ | addresstranslator(2), datamover(2), MMU(2), GMMU(2) |
| Multi-MW split (batch 3) | ✅ | writeback(2), writearound(2), writeevict(2), writethrough(2) — PR #51 |

### Per-Component Status

| Component | Comp Wrapper? | A-B Correct? | Multi-MW? | MW Count | Notes |
|-----------|:---:|:---:|:---:|:---:|---|
| idealmemcontroller | thin (StorageOwner) | ✅ | ✅ | 2 | Reference implementation |
| writearound cache | none | ✅ | ✅ | 2 | Done (M25) |
| writeevict cache | none | ✅ | ✅ | 2 | Done (M25) |
| writethrough cache | none | ✅ | ✅ | 2 | Done (M25) |
| writeback cache | none | ✅ | ✅ | 2 | Done (M25) |
| TLB | thin | ✅ | ✅ | 2 | Done |
| mmuCache | thin | ✅ | ✅ | 2 | Done |
| DRAM | none | ✅ | ✅ | 3 | Done (M23) |
| addresstranslator | none | ✅ | ✅ | 2 | Done (M24) |
| datamover | none | ✅ | ✅ | 2 | Done (M24) |
| simplebankedmemory | thin (StorageOwner) | ✅ | ✅ | 2 | Done (M23) |
| MMU | none | ✅ | ✅ | 2 | Done (M24) |
| GMMU | none | ✅ | ✅ | 2 | Done (M24) |
| endpoint | thin (API) | ✅ | ✅ | 2 | Done (M23) |
| switch | thin (API) | ✅ | ✅ | 2 | Done (M23) |
| tickingping | none | ✅ | ✅ | 2 | Done (M23) |

### Summary of Remaining Gaps

1. **component_guide.md** needs update for A-B state, multi-MW, no-dependency patterns
2. **VictimFinder interface + old Directory struct** still exist (unused dead code — not removed)
3. **queueing.Buffer adapters** still used by switch for arbitration compatibility
4. **examples/ping** still uses old event-driven model (not modeling.Component)
5. **Thin Comp wrappers** remain in 6 components for StorageOwner/API interfaces (acceptable per spec)

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

### ✅ M24: Multi-Middleware Split — AddressTranslator, DataMover, MMU, GMMU (DONE)
- Budget: 6 | Used: ~4 | PR: #50
- Split 4 components into multiple middlewares
- addresstranslator → 2 MW, datamover → 2 MW, MMU → 2 MW, GMMU → 2 MW

### ✅ M25: Multi-Middleware Split — Cache Components (DONE)
- Budget: 8 | Used: ~6 | PR: #51
- Split all 4 cache components into pipelineMW + controlMW
- writeback(2), writearound(2), writeevict(2), writethrough(2)

### M26: Final Cleanup + Documentation ⬅️ NEXT

**Goal:** Final pass — consistency, documentation, dead code removal.

1. **Update component_guide.md** to reflect the final multi-MW architecture with A-B state patterns
2. **Remove dead code**: VictimFinder interface, old Directory struct (unused since MSHR/Directory moved to State + free functions)
3. **Review examples/ping** — update to modeling.Component or document as legacy
4. **Review directconnection** — determine if it should use modeling.Component or stay as infrastructure
5. **Ensure all components** follow the identical pattern consistently
6. **Full test suite pass** + CI green
7. **Final code review pass**

**Budget**: 4 cycles

---

## Phase 2 Summary

| Milestone | Scope | Budget | Used | Status |
|-----------|-------|--------|------|--------|
| M21 | Cache cleanup | 8 | ~6 | ✅ Done |
| M21.5 | Fix CI lint failures | 2 | 1 | ✅ Done |
| M22 | Fix A-B + eliminate Comp | 6 | ~3 | ✅ Done |
| M23 | Multi-MW split (batch 1: 5 components) | 6 | ~5 | ✅ Done |
| M24 | Multi-MW split (batch 2: 4 non-cache) | 6 | ~4 | ✅ Done |
| M25 | Multi-MW split (batch 3: 4 caches) | 8 | ~6 | ✅ Done |
| M26 | Final cleanup + docs | 4 | — | ⬅️ NEXT |
| **Total Phase 2** | | **40** | **~25** | |

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
| M24 | 6 | ~4 | Multi-MW split — 4 non-cache components |
| M25 | 8 | ~6 | Multi-MW split — 4 cache components |

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
- **Multi-middleware split is mechanical** — M23 and M24 completed efficiently with one worker per component. Continue this pattern.
- **M23 needed a fix round** — flit metadata loss in endpoint/switch caught by verification. Always verify carefully.
- **Assign lint-checking to workers explicitly** — don't assume it happens automatically.
- **M24 completed in ~4 cycles** — under budget, validating the one-worker-per-component approach.
