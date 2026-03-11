# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware.

## Current State (after M17)

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works with acceptance test passing
- A-B state implemented in `modeling.Component` (double-buffered: current/next, deep-copy, swap)
- **14 components fully transformed** (Comp eliminated or thin wrapper + A-B state):
  1. idealmemcontroller (M12) — thin Comp wrapper for Storage
  2. TLB (M13) — thin Comp wrapper for PageTable
  3. mmuCache (M14) — thin Comp wrapper
  4. addresstranslator (M14) — thin Comp wrapper
  5. datamover (M14) — thin Comp wrapper
  6. simplebankedmemory (M14) — thin Comp wrapper
  7. GMMU (M15) — thin wrapper
  8. Switch (M15) — thin Comp wrapper
  9. Endpoint (M15) — thin Comp wrapper
  10. writearound cache (M16) — no Comp, returns `*modeling.Component` directly
  11. writeevict cache (M16) — no Comp, returns `*modeling.Component` directly
  12. writethrough cache (M16) — no Comp, returns `*modeling.Component` directly
  13. tickingping (M16) — no Comp, returns `*modeling.Component` directly
  14. writeback cache (M17) — no Comp, returns `*modeling.Component` directly
- Shared MSHR/Directory free functions created in `mem/cache/` (directory_ops.go, mshr_ops.go)
- Architecture direction fully clarified and approved by human (issues #145, #150)
- All PRs merged through #42. Code builds and all tests pass on main.

## Remaining Work

| Component | LOC | Dependencies to Inline | State Complexity | Planned |
|-----------|-----|----------------------|-----------------|---------|
| DRAM | ~4800 | AddrConverter, SubTransSplitter, AddrMapper, CmdQueue, Channel, Banks | Banks, channels, queues, timing | M18 |

After DRAM, only final cleanup/documentation remains (M19).

### M18: DRAM Memory Controller — Full Transformation (NEXT)

**Goal:** Transform the DRAM memory controller to match the established pattern. This is the last substantive component transformation.

**What to do:**
1. **Populate Spec** with ALL immutable config: Protocol, timing parameters (30+ tXXX values), bus/burst/device params, bank/rank/channel counts, queue sizes, address converter params, address mapping bit positions/masks. Timing table (currently computed in builder) should be computed once and stored in Spec.
2. **Eliminate Comp wrapper** → Builder returns `*modeling.Component[Spec, State]` (or thin wrapper for Storage only)
3. **Inline all 7 dependencies** into middleware:
   - `AddressConverter` → params in Spec, inline conversion logic (~15 LOC)
   - `SubTransSplitter` → `log2AccessUnitSize` in Spec, inline split logic (~25 LOC)
   - `AddressMapper` → bit position/mask params in Spec, inline mapping (~12 LOC)
   - `SubTransactionQueue` (FCFSSubTransactionQueue) → queue contents in State, logic in middleware (~60 LOC)
   - `CommandQueue` → queue contents in State, logic in middleware (~60 LOC)
   - `Channel + Banks` → bank states in State, timing in Spec, logic in middleware (~220 LOC)
   - `inflightTransactions` → already in State as `[]transactionState`, make canonical
4. **Remove internal/ packages** — flatten all logic into the dram package as middleware free functions
5. **Remove snapshot/restore conversion layer** (~495 LOC in state.go) — State IS the canonical representation
6. **Remove signal package** — Command/SubTransaction/Transaction become plain State structs
7. **Update all tests** to work with State + free functions

**Budget**: 8 cycles
**Risk**: Medium-High. Many internal packages but each is individually simple. Existing state.go already decomposes everything.

### M19: Final Cleanup + Documentation

**Goal:** Final verification, update component_guide.md, ensure all thin Comp wrappers are minimal, cleanup any remaining issues.

**Budget**: 4 cycles
**Risk**: Low.

## ✅ Completed Milestones

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

## Summary Statistics
- Total milestones completed: 17
- PRs merged: 42
- Components ported: 16/16
- Components fully transformed (Comp eliminated + A-B): 14/16 (DRAM remaining, plus thin wrappers are acceptable)
- Remaining to transform: 1 substantive (DRAM) + final cleanup

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
- Budget estimates are improving: M14 and M15 both finished well under budget (3 cycles each, budgets of 6 and 5)
- M16 finished in 4 cycles (budget 8). M17 finished in 3 cycles (budget 6).
- Shared free functions (directory_ops.go, mshr_ops.go) are reusable for writeback cache — reduces effort.
- DRAM has an existing state.go with complete snapshot/restore that proves the decomposition is structurally sound.
- **Revised estimation**: Complex components (writeback) take 3-4 cycles. DRAM may take more due to 5 internal packages.
