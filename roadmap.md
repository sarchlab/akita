# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware.

## Current State (after M16)

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works with acceptance test passing
- A-B state implemented in `modeling.Component` (double-buffered: current/next, deep-copy, swap)
- **13 components fully transformed** (Comp eliminated + A-B state):
  1. idealmemcontroller (M12)
  2. TLB (M13)
  3. mmuCache (M14)
  4. addresstranslator (M14)
  5. datamover (M14)
  6. simplebankedmemory (M14)
  7. GMMU (M15)
  8. Switch (M15)
  9. Endpoint (M15)
  10. writearound cache (M16)
  11. writeevict cache (M16)
  12. writethrough cache (M16)
  13. tickingping (M16)
- Shared MSHR/Directory free functions created in `mem/cache/` (directory_ops.go, mshr_ops.go)
- Architecture direction fully clarified and approved by human (issues #145, #150)
- All PRs merged through #41. Code builds and all tests pass on main.

## Remaining Components

| Component | LOC | Dependencies to Inline | State Complexity | Planned |
|-----------|-----|----------------------|-----------------|---------|
| writearound cache | ~2250 | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory sets, MSHR, pipelines, buffers | M16 |
| writeevict cache | ~2250 | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory sets, MSHR, pipelines, buffers | M16 |
| writethrough cache | ~2250 | Directory, MSHR, Storage, AddrToPortMapper, AddrConverter | Transactions, directory sets, MSHR, pipelines, buffers | M16 |
| writeback cache | ~3200 | Directory, MSHR, Storage, AddrToPortMapper, VictimFinder, AddrConverter | 60+ fields, 6 internal stages | M17 |
| DRAM | ~4800 | AddrConverter, SubTransSplitter, AddrMapper, CmdQueue, Channel, Banks | Banks, channels, queues, timing | M18 |
| tickingping (example) | trivial | None | Simple counter | M16 (bundled) |

### Key Insight: The 3 Simpler Caches Are Nearly Identical

writearound, writeevict, and writethrough share identical Comp structs, identical state.go files, and nearly identical stage implementations. They differ primarily in directory.go (write policy) and small details in bottom parser/coalescer. This means:
- Transform ONE cache fully as a template
- Apply the same pattern to the other two with minimal changes
- Common cache utilities (MSHR free functions, Directory free functions) can be shared via the `mem/cache/` package

## Phase: Cache Architecture Transformation

### ~~M16: Write{around,evict,through} Caches + tickingping~~ ✅ DONE (4 cycles, budget 8)

Transformed all 3 simpler caches + tickingping. Created shared MSHR/Directory free functions. PR #41 merged.

### M17: Writeback Cache — Full Transformation (NEXT)

**Goal:** Transform the most complex cache component following the M16 pattern.

**What to do:**
1. Populate Spec with immutable config: `NumReqPerCycle`, `Log2BlockSize`, `BankLatency`, `WayAssociativity`, `NumBanks`, `NumSets`, `NumMSHREntry`, `TotalByteSize`, `DirLatency`, `WriteBufferCapacity`, `MaxInflightFetch`, `MaxInflightEviction`
2. Eliminate Comp wrapper → Builder returns `*modeling.Component[Spec, State]`
3. Reuse shared MSHR/Directory free functions from `mem/cache/` (directory_ops.go, mshr_ops.go)
4. Replace all `cache.Directory` calls with free function calls on `state.DirectoryState`
5. Replace all `cache.MSHR` calls with free function calls on `state.MSHRState`
6. Inline AddressToPortMapper: store port names in Spec, resolve via `GetPortByName()`
7. 6 existing stages (topParser, directoryStage, bankStage, writeBufferStage, mshrStage, flusher) each become separate middleware
8. Remove ~964 LOC snapshot/restore conversion layer. State IS the canonical representation
9. Eliminate all runtime `*transaction` pointer chains → index-based `transactionState` in State
10. Remove `SaveState/LoadState` overrides — `modeling.Component` handles A-B state

**Complexity notes:**
- 3150 LOC production code, ~3000 LOC tests
- 6 stages (vs 5 for simpler caches) + writeBufferStage + mshrStage are unique to writeback
- `directory.GetSets()` used in flusher — need DirectoryGetSets free function
- VictimFinder already uses LRU queue in DirectoryState (no separate dependency)
- evictingList map needs to be in State (already is)

**Budget**: 6 cycles (pattern well-established from M16; the main challenge is the extra stages and test rewriting)
**Risk**: Medium-High. More complex than the simpler caches but pattern is proven.

### M18: DRAM Memory Controller

**Goal:** Transform DRAM with its multiple internal packages.

**What to do:**
1. Inline all 7 dependencies: AddressConverter, SubTransSplitter, AddressMapper, CommandQueue, Channel, Banks
2. Bank state, channel state, queue contents → State (serialization code already does this decomposition)
3. Timing tables (~200 entries) → Spec
4. Remove internal/ package structure — flatten into middleware
5. Use A-B state for all mutable data

**Budget**: 8 cycles
**Risk**: Medium-High. Many internal packages but each is individually simple. Existing state.go already decomposes everything.

### M19: Multi-Middleware Split + Final Cleanup

**Goal:** Split any remaining single-middleware components into multiple middlewares. Update component creation guide. Final cleanup.

**Budget**: 6 cycles
**Risk**: Low. By this point all patterns are well-established.

### M20: Project Completion

**Goal:** Final verification, documentation, and sign-off.
- All components use `modeling.Component[Spec, State]` directly
- No Comp wrappers (except thin StorageOwner wrappers)
- All dependencies inlined
- A-B state everywhere
- Save/load acceptance test passes
- CI green
- component_guide.md updated to reflect final architecture

**Budget**: 2 cycles

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

## Summary Statistics
- Total milestones completed: 16
- PRs merged: 41
- Components ported: 16/16
- Components fully transformed (Comp eliminated + A-B): 13/16
- Remaining to transform: 3 (writeback cache + DRAM + multi-MW split/cleanup)

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
- **Revised estimation**: Simpler milestones need 3-4 cycles, not 5-6. Bump cache milestones accordingly.
- M16 finished in 4 cycles (budget 8). Multi-worker parallel approach worked well for cache replication.
- Shared free functions (directory_ops.go, mshr_ops.go) are reusable for writeback cache — reduces M17 effort.
