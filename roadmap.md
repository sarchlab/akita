# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware, make State canonical (no runtime copies), split monolithic middlewares into multiple stages.

## Current State (after M21, Cycle 176)

### Phase 1 COMPLETE: Component Model + A-B State + Comp Elimination + Dependency Inlining

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works with acceptance test passing
- A-B state implemented in `modeling.Component` (double-buffered: current/next, deep-copy, swap)
- DRAM fully transformed: internal packages eliminated, all dependencies inlined as free functions
- Caches transformed: MSHR/Directory as State + free functions, no Comp wrappers, A-B state applied
- MMU fully transformed: thin Comp, transactionState canonical, no SaveState/LoadState overrides
- Switch fully transformed: State canonical, queueing objects eliminated, no conversion layers

### M21 COMPLETE: Cache Components — Eliminate Runtime Copies + Inline Dependencies
- All 4 caches (writearound, writeevict, writethrough, writeback) cleaned up
- Runtime copies of DirectoryState/MSHRState eliminated
- queueing.Pipeline/Buffer replaced with State arrays + free functions (adapters.go)
- addressToPortMapper inlined (legacyMapper resolved at Build time)
- restoreFromState eliminated
- A-B state pattern applied (GetState for read, GetNextState for write)
- PR #46 merged to main

### ⚠️ CI FAILING on main (25 lint errors from M21 merge)
- **funlen** (12): Functions too long in cache builders, adapters, writeback stages, DRAM builder, MMU test
- **gocognit** (6): dirPipelineTick/bankPipelineTick too complex in 3 cache adapters
- **unused** (5): finalizeMSHRTrans in 3 caches, stringToCmdKind in DRAM, flitStateFromFlit in endpoint
- **lll** (1): Line too long in virtualmem acceptance test
- **unconvert** (1): Unnecessary conversion in DRAM builder

### Summary Statistics

| Metric | Value |
|--------|-------|
| Milestones completed | 21 (M1–M21) |
| Total cycles used | ~100 |
| PRs merged | 46 |
| Components ported | 16/16 |
| Components fully transformed (State canonical) | 16/16 |
| CI status | ❌ FAILING (lint) |

### Remaining Gaps (Phase 2 scope)

| Gap | Components | Description | Priority |
|-----|-----------|-------------|----------|
| **Cache runtime copies** | writeback, writearound, writeevict, writethrough | Middleware holds runtime copies of DirectoryState/MSHRState, queueing.Buffer/Pipeline objects, addressToPortMapper dependency, restoreFromState layers | HIGH |
| **DRAM A-B pattern** | mem/dram | Uses `GetNextState()` for both read and write — should use `GetState()` for read, `GetNextState()` for write | MEDIUM |
| **Monolithic middleware** | 13/16 components | All but idealmemcontroller, TLB, mmuCache have exactly 1 middleware. Should be multiple. | HIGH |
| **component_guide.md** | docs | Needs update to reflect final architecture | LOW |
| **directconnection** | sim/directconnection | Not using modeling.Component (uses TickingComponent). Infrastructure — may be intentional. | LOW |
| **examples/ping** | examples/ping | Uses ComponentBase directly. Example code. | LOW |

---

## Phase 2: Cache Cleanup + A-B Pattern + Multi-Middleware Split

### ✅ M21: Cache Components — Eliminate Runtime Copies + Inline Dependencies (DONE)

**Completed**: All 4 caches cleaned up. Runtime copies eliminated, queueing objects replaced with State arrays + free functions (adapters.go), addressToPortMapper inlined (legacyMapper at Build time), restoreFromState removed, A-B state applied.
**Budget**: 8 cycles | **Used**: ~6 cycles | **PR**: #46 merged

### M21.5: Fix CI Lint Failures on Main (URGENT)

**Goal:** Fix all 25 lint errors introduced by M21 merge so CI passes again.

**Errors to fix:**
1. **funlen** (12): Split long functions — cache Build(), buildAdapters(), directorystage fetch(), writeback state.go snapshot/restore, DRAM buildSpec()/buildAddressMapping(), MMU state_test.go
2. **gocognit** (6): Reduce complexity of dirPipelineTick/bankPipelineTick in writeback/writeevict/writethrough adapters.go — extract helper functions
3. **unused** (5): Remove finalizeMSHRTrans from writearound/writeevict/writethrough bottomparser.go, stringToCmdKind from DRAM state.go, flitStateFromFlit from endpoint.go
4. **lll** (1): Shorten long line in virtualmem/test.go
5. **unconvert** (1): Remove unnecessary Protocol() conversion in DRAM builder.go

**Budget**: 2 cycles
**Risk**: Low. Mechanical fixes.

### M22: DRAM + Simple Components — Fix A-B Pattern + Inline Remaining Dependencies

**Goal:** Fix the DRAM A-B pattern issue. Verify all simple components (addresstranslator, datamover, simplebankedmemory, endpoint, tickingping) have correct A-B usage. Inline any remaining dependency interfaces.

**What to do:**
1. **DRAM**: Change `GetNextState()` reads to `GetState()` reads, keep `GetNextState()` for writes.
2. **Audit all components** for A-B correctness: every `Tick()` should use `GetState()` for read, `GetNextState()` for write.
3. Inline any remaining dependency interfaces found during audit.
4. Verify save/load acceptance test still passes.

**Budget**: 4 cycles
**Risk**: Low. The A-B change is mechanical. DRAM behavior may change slightly (reads old values instead of current writes) but this is the intended semantics.

### M23: Multi-Middleware Split — Simple Components

**Goal:** Split the simpler single-middleware components into multiple middlewares following natural stage boundaries.

**Target components and proposed splits:**
1. **addresstranslator** → 2 MW: parse incoming, send response
2. **datamover** → 2 MW: control, data transfer
3. **simplebankedmemory** → 2 MW: control, memory operations
4. **endpoint** → 2 MW: incoming processing, outgoing processing
5. **tickingping** → 2 MW: receive, send
6. **DRAM** → 3-5 MW: parseTop, subTransQueue, bankTick, issue, respond

**Each split must:**
- Maintain correct A-B state semantics (read current, write next)
- Not break save/load (State struct unchanged)
- Pass existing tests
- Document the middleware boundary rationale

**Budget**: 6 cycles
**Risk**: Low-Medium. These are simpler components. DRAM is the most complex here but already has logical separation in its Tick() function.

### M24: Multi-Middleware Split — Cache Components

**Goal:** Split the 4 cache components into multiple middlewares.

**Proposed middleware boundaries (based on Diana's analysis):**
- **writearound**: topParser → directory → bank → bottomParser → respond (5 stages)
- **writeevict**: same structure as writearound
- **writethrough**: same structure
- **writeback**: topParser → directory → bank → writeBuffer → mshr → flusher (6 stages)

**Key considerations:**
- +1 cycle latency per middleware boundary — compensate via Spec timing constants if needed
- Each stage reads from `current`, writes to `next` — stages don't see each other's writes
- Shared free functions (directory_ops, mshr_ops) work with State by index — already compatible

**Budget**: 8 cycles
**Risk**: High. Cache split is the most complex change. The writeback cache has the most stages and complex inter-stage data flow.

### M25: Multi-Middleware Split — Complex Components (MMU, Switch, TLB, mmuCache)

**Goal:** Split remaining components into multiple middlewares.

1. **TLB** — already has 2 MW, verify A-B correctness
2. **mmuCache** — already has 2 MW, verify A-B correctness
3. **MMU** → 3-4 MW: parseFromTop, walkPageTable, migration, respond
4. **Switch** → multiple MW per pipeline stage (route, forward, sendOut per port complex)

**Budget**: 6 cycles
**Risk**: Medium. MMU and Switch are complex but TLB/mmuCache may only need verification.

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
**Risk**: Low.

---

## Phase 2 Summary

| Milestone | Scope | Budget | Status |
|-----------|-------|--------|--------|
| M21 | Cache cleanup (runtime copies, queueing, deps) | 8 | ✅ Done (~6 used) |
| M21.5 | Fix CI lint failures | 2 | ⬅️ NEXT |
| M22 | DRAM A-B fix + audit all components | 4 | Pending |
| M23 | Multi-MW split — simple components | 6 | Pending (after M22) |
| M24 | Multi-MW split — cache components | 8 | Pending (after M21, M23) |
| M25 | Multi-MW split — complex components | 6 | Pending (after M23) |
| M26 | Final cleanup + docs | 4 | Pending (after M24, M25) |
| **Total** | | **38** | |

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

**Phase 1 totals**: Budget: 150, Used: 94 (37% under budget)
**Phase 2 so far**: M21: Budget 8, Used ~6

## Lessons Learned

- CI can get stuck in "queued" state — don't waste cycles waiting for it
- Architecture discussions should be fully resolved before implementation
- Multi-worker mechanical changes work well with clear patterns
- Breaking milestones to 2-6 cycle budgets is optimal
- Human feedback drives direction — stay responsive
- Combined milestones work when scope is small
- idealmemcontroller is the reference implementation — follow its patterns
- The snapshot/restore conversion layer disappears when State is canonical (big code reduction)
- A-B state deep copy via JSON round-trip is acceptable for small States
- Components with external services (Storage, PageTable, RoutingTable) keep those as middleware fields
- The 3 simpler caches are nearly identical — transform one, replicate twice
- Budget estimates are improving: most milestones finish well under budget
- Shared free functions (directory_ops.go, mshr_ops.go) are reusable across cache types
- DRAM has an existing state.go with complete snapshot/restore that proves decomposition is structurally sound
- **Multi-middleware split is the next major architectural change** — needs careful planning per component
- **Reference implementations matter**: establish the pattern on simple components first, then replicate
- **M21 and M22 can run in parallel** since they affect different components
- **Always run lint before merging**: M21 introduced 25 lint errors on main. CI-fix milestones waste cycles. Ares must run linter before claiming complete.
- **CI fix milestones should be tiny** (1-2 cycles): they're mechanical and should never block feature work for long
