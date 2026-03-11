# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware.

## Current State (after M15)

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works with acceptance test passing
- A-B state implemented in `modeling.Component` (double-buffered: current/next, deep-copy, swap)
- **9 components fully transformed** (Comp eliminated + A-B state):
  1. idealmemcontroller (M12)
  2. TLB (M13)
  3. mmuCache (M14)
  4. addresstranslator (M14)
  5. datamover (M14)
  6. simplebankedmemory (M14)
  7. GMMU (M15)
  8. Switch (M15)
  9. Endpoint (M15)
- Architecture direction fully clarified and approved by human (issues #145, #150)
- All PRs merged through #40. Code builds and all tests pass on main.

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

### M16: Write{around,evict,through} Caches + tickingping (NEXT)

**Goal:** Transform the 3 simpler cache types + tickingping. Since they're nearly identical, do one fully then replicate.

**What to do for each cache:**
1. Eliminate Comp wrapper → `modeling.Component[Spec, State]`
2. Move immutable config to Spec: `numReqPerCycle`, `log2BlockSize`, `bankLatency`, `wayAssociativity`, `maxNumConcurrentTrans`
3. Inline AddressToPortMapper: store port names in Spec, resolve via `GetPortByName()`
4. Inline AddressConverter: interleaving params to Spec, conversion logic directly in middleware
5. Eliminate `cache.Directory` dependency: directory data already in `State.DirectoryState`, add free functions for Lookup/FindVictim/Visit that operate on State
6. Eliminate `cache.MSHR` dependency: MSHR data already in `State.MSHRState`, add free functions for Query/Add/Remove that operate on State
7. Each stage becomes its own middleware reading `GetState()` / writing `GetNextState()`
8. Remove snapshot/restore conversion layer (state.go ~610 LOC each)
9. All runtime `*transaction` slices → use State transaction indices

**MSHR/Directory free functions** (in `mem/cache/` package, shared by all caches):
- `MSHRQuery(state *MSHRState, pid sim.PID, addr uint64) *MSHREntryState`
- `MSHRAdd(state *MSHRState, capacity int, pid sim.PID, addr uint64) (*MSHREntryState, error)`
- `MSHRRemove(state *MSHRState, pid sim.PID, addr uint64)`
- `MSHRIsFull(state *MSHRState, capacity int) bool`
- `DirectoryLookup(state *DirectoryState, pid sim.PID, addr uint64, blockSize uint64) *BlockState`
- `DirectoryEvict(state *DirectoryState, setID int) *BlockState` (LRU)
- `DirectoryVisit(state *DirectoryState, setID int, wayID int)`

**tickingping:** Trivial — just eliminate Comp wrapper, move config to Spec.

**Budget**: 8 cycles
**Risk**: Medium. The pattern is established but cache stages have complex transaction flow. The near-identical nature of the 3 caches reduces risk (do one, replicate twice).

### M17: Writeback Cache — Full Transformation

**Goal:** Transform the most complex cache component.

**What to do:**
1. Reuse MSHR/Directory free functions from M16
2. Eliminate Comp wrapper
3. Inline VictimFinder (LRU — use LRU queue already in DirectoryState)
4. 6 existing stages (topParser, directoryStage, bankStage, writeBufferStage, mshrStage, flusher) each become separate middleware
5. Remove ~964 LOC snapshot/restore conversion layer
6. Eliminate all runtime `*transaction` pointer chains → index-based State

**Budget**: 8 cycles
**Risk**: High. Writeback cache has the most complex state and stage interactions.

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

## Summary Statistics
- Total milestones completed: 15
- PRs merged: 40
- Components ported: 16/16
- Components fully transformed (Comp eliminated + A-B): 9/16
- Remaining to transform: 7 (4 caches + DRAM + tickingping + possibly writeback needs multi-MW split)

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
