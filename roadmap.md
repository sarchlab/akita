# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware.

## Current State

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works
- CI runs stuck in "queued" (GitHub Actions runner issue, not code issue)
- All PRs merged (#28, #29, #30, #31). Code compiles and tests pass locally.
- Architecture direction fully clarified by human (issues #145, #150)
- Human said "It seems we have a clear plan. Let's go!" (issue #145)
- Detailed analysis complete: Diana (A-B state co-design), Iris (dependency elimination)

## Phase: Architecture Transformation

### M10: CI fix + Merge PR #31 ✅ (DONE)
- PR #31 merged (funlen fix). Dependabot PRs merged. Code builds & tests pass.
- CI runners stuck in "queued" — not a code issue.
- **Budget**: 2 → **Used**: 3 (failed due to CI queue, but work was done)

### M11: Finalize Architecture Design ✅ (DONE)
- Human approved A-B state (#150), Comp elimination (#145), no dependencies
- Diana and Iris produced detailed analysis with code examples
- Human said "Let's go!" — no further discussion needed
- **Budget**: 2 → **Used**: 0 (completed as part of M10 cycles)

### M12: A-B State + Comp Elimination on idealmemcontroller (NEXT)
- Combined M12+M13 since idealmemcontroller is small enough to do both at once
- Changes to `modeling.Component`: add current/next state, deep copy, swap-after-tick
- Changes to `modeling/saveload.go`: serialize only current state
- Eliminate `Comp` wrapper in idealmemcontroller
- Embed AddressConverter logic directly in middleware
- Middleware holds `*mem.Storage` as sole external reference
- Builder returns `*modeling.Component[Spec, State]` (or a thin interface wrapper for StorageOwner)
- All existing tests must pass (may need updating for new API)
- **Budget**: 5 cycles

### M14: Comp Elimination on TLB
- Remove Comp wrapper from TLB
- Embed all dependencies in middleware
- Middleware reads from A-buffer, writes to B-buffer
- **Budget**: 5 cycles

### M15: MSHR/Directory Decoupling + Comp Elimination on Writeback Cache
- MSHR data → State, behavior → free functions
- Directory data → State, behavior → free functions
- Eliminate Comp wrapper, embed all dependencies
- **Budget**: 8 cycles

### M16: Comp Elimination on Remaining Components
- Apply pattern to DRAM, switch, endpoint, datamover, etc.
- **Budget**: 8 cycles

### M17: Multi-Middleware Split
- Split single-middleware components into multiple middlewares
- Each middleware operates on A-B state independently
- **Budget**: 10 cycles

## ✅ Previous Milestones Complete

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
| M10 | 2 | 3* | CI fix + Dependabot |
| M11 | 2 | 0 | Architecture design (done in discussion) |

## Summary Statistics
- Total milestones completed: 11
- PRs merged: 31
- Components ported: 16/16

## Lessons Learned
- CI can get stuck in "queued" state — don't waste cycles waiting for it
- Architecture discussions should be fully resolved before implementation
- Multi-worker mechanical changes work well
- Breaking milestones to 2-6 cycle budgets is optimal
- Human feedback drives direction — stay responsive
- When CI is blocked, focus on other productive work
- Combined milestones work when scope is small (idealmemcontroller has only ~10 State fields, 2 middleware, 1 dependency)
