# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware, split monolithic middlewares into multiple stages.

## Current State (after M18)

### Phase 1 COMPLETE: Component Model + A-B State + Comp Elimination + Dependency Inlining

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works with acceptance test passing
- A-B state implemented in `modeling.Component` (double-buffered: current/next, deep-copy, swap)
- DRAM fully transformed: internal packages eliminated, all dependencies inlined as free functions
- Caches transformed: MSHR/Directory as State + free functions, no Comp wrappers
- All builds/tests pass on main

### Remaining Gaps (to be addressed in Phase 2)

| Issue | Components Affected | Description |
|-------|-------------------|-------------|
| Monolithic middleware | ALL 16 components | Every component has exactly 1 middleware. Human says this is historical; should be multiple. |
| MMU thick Comp wrapper | `mem/vm/mmu` | Runtime transactions, page access tracking on Comp (not in State). SaveState/LoadState conversion layer still exists. |
| Switch runtime objects | `noc/networking/switching/switches` | Pipeline/buffer runtime objects in middleware. SaveState/LoadState sync/restore layer. |
| Endpoint runtime objects | `noc/networking/switching/endpoint` | Buffer runtime objects in middleware (less severe than Switch). |
| DRAM A-B pattern | `mem/dram` | Uses GetNextState() for both read and write in Tick() — should read GetState(), write GetNextState(). |
| directconnection | `sim/directconnection` | Not using modeling.Component at all (uses TickingComponent directly). May be intentional — it's infrastructure, not a simulation component. |
| examples/ping | `examples/ping` | Uses ComponentBase directly. Example/demo code — may not need transformation. |
| component_guide.md | docs | Needs update to reflect final architecture (multi-middleware, A-B state, no Comp pattern). |

## Phase 2 Milestones

### M19: MMU Full Transformation

**Goal:** Transform the MMU component to eliminate the thick Comp wrapper and make State canonical.

**What to do:**
1. Move all runtime fields from Comp to State (walkingTranslations, migrationQueue, currentOnDemandMigration, isDoingMigration, toRemoveFromPTW, PageAccessedByDeviceID, nextPhysicalPage)
2. Eliminate the runtime `transaction` type — use `transactionState` as the canonical type
3. Remove SaveState/LoadState overrides and the sync/restore conversion layer (~85 LOC)
4. Keep `vm.PageTable` as an external service reference on middleware (like Storage)
5. Make middleware use GetState()/GetNextState() correctly for A-B pattern
6. Update tests

**Budget**: 4 cycles
**Risk**: Medium. MMU has complex migration logic but the state decomposition already exists in transactionState/pageState.

### M20: Switch & Endpoint — Make State Canonical

**Goal:** Transform Switch and Endpoint to eliminate runtime pipeline/buffer objects, make State canonical, remove SaveState/LoadState conversion layers.

**What to do:**
1. **Switch**: Replace `queueing.Pipeline` and `queueing.Buffer` runtime objects with State arrays. Pipeline tick/accept and buffer push/pop become free functions on State. Keep `routing.Table` and `arbitration.Arbiter` as external service references on middleware.
2. **Switch**: Remove syncToState/restoreFromState and SaveState/LoadState overrides.
3. **Endpoint**: Replace runtime buffer objects with State arrays if any remain. Remove conversion layers.
4. Fix A-B state pattern: GetState() for read, GetNextState() for write.
5. Update tests.

**Budget**: 5 cycles
**Risk**: Medium-High. Switch has complex pipeline/buffer structures with multiple port complexes. The portComplex State type already exists but needs to become canonical.

### M21: Multi-Middleware Split — Reference Implementation

**Goal:** Split the idealmemcontroller (which already has 2 middlewares) into a clean reference, then split 2-3 additional simple components to establish the pattern.

**What to do:**
1. Verify idealmemcontroller already properly demonstrates multi-middleware with A-B state
2. Split addresstranslator into 2 middlewares (parse + respond)
3. Split datamover into 2 middlewares (ctrl + data transfer)
4. Split simplebankedmemory into 2 middlewares (ctrl + memory operations)
5. Ensure all use correct A-B state pattern
6. Verify save/load still works
7. Document the multi-middleware pattern in component_guide.md

**Budget**: 5 cycles
**Risk**: Low-Medium. These are simple components; the split is mostly mechanical.

### M22: Multi-Middleware Split — Cache Components

**Goal:** Split the 4 cache components into multiple middlewares following the established pattern.

**What to do:**
1. Design cache middleware boundaries (Diana's analysis suggested 6 stages for writeback: topParser → directory → bank → writeBuffer → mshr → flusher)
2. Split writearound cache first (simplest cache) — establish cache multi-middleware pattern
3. Replicate to writeevict, writethrough
4. Split writeback cache (most complex — 6+ stages)
5. Verify +1 cycle latency per boundary is acceptable (adjust Spec timing constants if needed)
6. Verify save/load still works

**Budget**: 8 cycles
**Risk**: High. Cache split is the most complex change. Each stage must read from current and write to next without seeing other stages' writes. Need careful testing.

### M23: Multi-Middleware Split — Remaining Components

**Goal:** Split remaining components: TLB, mmuCache, MMU, Switch, Endpoint, DRAM.

**What to do:**
1. TLB already has 2 middlewares — verify A-B correctness
2. mmuCache already has 2 middlewares — verify A-B correctness
3. Split MMU into multiple middlewares (parseFromTop, walkPageTable, migration, respond)
4. Split Switch into multiple middlewares per pipeline stage
5. Split Endpoint into multiple middlewares
6. Split DRAM into multiple middlewares (parseTop, subTransQueue, issue, bankTick, respond)
7. Verify all save/load works

**Budget**: 8 cycles
**Risk**: Medium-High. MMU and Switch are complex but patterns established by M21-M22 should help.

### M24: Final Cleanup + Documentation

**Goal:** Final verification, update all documentation, ensure consistency across all components.

**What to do:**
1. Update component_guide.md to reflect final architecture
2. Ensure all components follow the same pattern consistently
3. Fix any remaining A-B state pattern issues (GetState for read, GetNextState for write)
4. Clean up any remaining thin Comp wrappers that can be eliminated
5. Verify full test suite and acceptance tests pass
6. Review directconnection — determine if it should use modeling.Component or stay as infrastructure
7. Final code review pass

**Budget**: 4 cycles
**Risk**: Low.

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
| M14 | 6 | 3 | Simple Components Batch (mmuCache, addresstranslator, datamover, simplebankedmemory) |
| M15 | 5 | 3 | GMMU + Switch + Endpoint — Comp elimination + A-B state |
| M16 | 8 | 4 | Write{around,evict,through} caches + tickingping — Comp elimination + shared free functions |
| M17 | 6 | 3 | Writeback cache — Full transformation |
| M18 | 8 | 3 | DRAM memory controller — Full transformation |

## Summary Statistics
- Total milestones completed: 18
- Total cycles used: 88 (budgeted: 142)
- PRs merged: 43
- Components ported: 16/16
- Components fully transformed (Comp eliminated + dependencies inlined): 14/16 (MMU and Switch have remaining gaps)
- Components with multi-middleware: 3/16 (idealmemcontroller, TLB, mmuCache — all others have 1)
- Phase 2 estimated: 30 cycles across 6 milestones

## Lessons Learned
- CI can get stuck in "queued" state — don't waste cycles waiting for it
- Architecture discussions should be fully resolved before implementation
- Multi-worker mechanical changes work well with clear patterns
- Breaking milestones to 2-6 cycle budgets is optimal
- Human feedback drives direction — stay responsive
- Combined milestones work when scope is small — M14 (4 components) done in 3 cycles
- idealmemcontroller is the reference implementation — follow its patterns
- The snapshot/restore conversion layer disappears when State is canonical (big code reduction)
- A-B state deep copy via JSON round-trip is acceptable for small States
- Lint errors from multi-branch merges should be caught BEFORE merging to main
- Components with external services (Storage, PageTable, RoutingTable) keep those as middleware fields
- The 3 simpler caches are nearly identical — transform one, replicate twice
- Budget estimates are improving: most milestones finish well under budget
- Shared free functions (directory_ops.go, mshr_ops.go) are reusable across cache types
- DRAM has an existing state.go with complete snapshot/restore that proves decomposition is structurally sound
- **Multi-middleware split is the next major architectural change** — needs careful planning per component
- **Reference implementations matter**: establish the pattern on simple components first, then replicate
