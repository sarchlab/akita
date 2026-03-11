# Roadmap

## Project Goal

Evolve Akita V5: redefine component model, implement save/load, make messages plain structs, port all first-party components, and continuously refine the architecture toward cleaner abstractions.

## Current Phase: Architecture Discussion + CI Fix

Three human issues require attention:

### M9.1: Fix CI Failures (IMMEDIATE)

Human issue #151: CI is failing due to lint errors — unused variables in `v5/mem/mem/protocol.go`. Quick fix needed.

**Budget**: 2 cycles

### M9.2: Merge Dependabot PRs (IMMEDIATE)

Human issue #152: 6 open Dependabot PRs for npm dependency updates.

**Budget**: 2 cycles

### M9.3: Architecture Design — A-B State + Comp Elimination (DISCUSSION)

Two related architectural discussions happening in parallel:
- **#145**: Comp elimination — human asking about MSHR decoupling and dependency injection
- **#150**: A-B state (double-buffered state) — new proposal for digital-circuit-style state management

These discussions need analysis and response before any implementation milestone can be defined. Currently gathering analysis from workers (Diana on #150, Iris on #145).

**Budget**: 2-4 cycles (analysis and discussion only)

### M10: Implement Architecture Changes (PENDING DISCUSSION)

After #145 and #150 discussions converge, define implementation milestones. Likely sub-milestones:
- M10.1: A-B state in modeling.Component
- M10.2: MSHR/Directory data-behavior decoupling
- M10.3: Dependency injection pattern
- M10.4: Comp elimination (per-component)

**Estimated budget**: 20-30 cycles (TBD after discussion)

## ✅ Previous Milestones Complete

### M1: Create `modeling` package with Component struct ✅ — Budget: 6, Used: 5
### M2: Refactor `idealmemcontroller` to use modeling package ✅ — Budget: 6, Used: 4
### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8, Used: 6
### M4: Fix CI lint failures ✅ — Budget: 3, Used: 2
### M5: Redesign Messages as Plain Structs ✅ — Budget: 8, Used: 6
### M6: Port All First-Party Components ✅ — Budget: 16, Used: 8
### M7: Move Mutable Runtime Data into State Structs ✅ — Budget: 30, Used: 16
### M8: Msg-as-Interface Redesign ✅ — Budget: 24, Used: 18
### M9: Write Component Creation Guide ✅ — Budget: 4, Used: 2

## Summary Statistics
- **Total implementation cycles**: ~53 across 108 orchestrator cycles
- **Total milestones**: 9 root milestones (M1–M9), 12 sub-milestones
- **PRs merged**: 27
- **Components fully ported**: 16/16 with serializable State
- **Concrete message types**: 30+
- **Builder types removed**: ~22

## Lessons Learned
- Multi-worker approach for mechanical changes works very well (M6, M7, M8)
- Human feedback drives direction changes — stay responsive to discussion issues
- Apollo's verification catches real issues — always verify
- Breaking large milestones into sub-milestones with 2-6 cycle budgets is optimal
- Architecture discussions should be fully resolved before starting implementation
- CI regressions should be fixed immediately before they accumulate
