# Roadmap

## Project Goal

Evolve Akita V5 toward a clean component model: Component = Spec + State + Ports + Middleware + Hooks. Implement A-B state, eliminate Comp wrappers, eliminate external dependencies, embed all logic in middleware.

## Current State

- 16 first-party components ported to `modeling.Component[Spec, State]`
- Messages are concrete types (no builders)
- Save/load works
- CI blocked by funlen lint error — PR #31 ready, CI queued
- Dependabot PRs merged
- Architecture direction clarified by human (issues #145, #150)

## Phase: Architecture Transformation

### M10: Fix CI + Merge PR #31 (IMMEDIATE)
- Merge PR #31 (funlen fix for writeback/state.go)
- Verify CI passes on main
- **Budget**: 2 cycles

### M11: Discussion Response — Finalize Architecture Design
- Respond to human's latest questions on #145 (no dependencies, embed logic) and #150 (A-B state OK with 1-cycle delay)
- Produce a concrete design document showing the target component architecture
- Need to address: what does "embed logic directly in middleware" mean for each current dependency?
- **Budget**: 2 cycles (analysis only)

### M12: A-B State in modeling.Component
- Add current/next State to `modeling.Component`
- Implement swap-after-tick
- Update GetState/SetState semantics
- Proof of concept on idealmemcontroller (simplest multi-middleware component)
- **Estimated budget**: 4-6 cycles

### M13: Prototype Comp Elimination on idealmemcontroller
- Remove Comp wrapper from idealmemcontroller
- Move all mutable data to State
- Embed dependency logic (AddressConverter) directly in middleware
- Builder returns `*modeling.Component[Spec, State]`
- **Estimated budget**: 4-6 cycles

### M14: MSHR/Directory Data-Behavior Decoupling
- MSHR data → State, behavior → free functions called by middleware
- Directory data → State, behavior → free functions
- Start with writeback cache as reference implementation
- **Estimated budget**: 6-10 cycles

### M15: Eliminate Dependencies Across All Components
- Replace AddressToPortMapper with inline logic in middleware
- Replace VictimFinder with inline logic
- Replace AddressConverter with inline logic
- Replace Storage interface with direct state access
- **Estimated budget**: 8-12 cycles

### M16: Eliminate All Comp Wrappers
- Apply Comp elimination to all 16 components
- Each component's builder returns `*modeling.Component[Spec, State]`
- **Estimated budget**: 12-20 cycles

### M17: Split Single-Middleware Components into Multiple Middlewares
- Writeback cache: split pipeline stages into separate middlewares
- Other caches similarly
- Each middleware operates on A-B state independently
- **Estimated budget**: 10-16 cycles

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
| M9.1-M9.2 | 3 | 3* | CI fix + Dependabot (PR #31 ready, CI queued) |

*M9.1-M9.2 used 3 cycles. Work is done (PR #31 + Dependabot merged) but CI was queued so couldn't be verified/merged.

## Summary Statistics
- Total milestones completed: 9 root + 12 sub-milestones
- PRs merged: 30
- Components ported: 16/16

## Lessons Learned
- CI can get stuck in "queued" state — don't waste cycles waiting for it
- Architecture discussions should be fully resolved before implementation
- Multi-worker mechanical changes work well
- Breaking milestones to 2-6 cycle budgets is optimal
- Human feedback drives direction — stay responsive
- When CI is blocked, focus on other productive work (analysis, design) instead of waiting
