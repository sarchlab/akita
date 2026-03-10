# Roadmap

## M1: Create `v5/queueing/` package with Buffer and Pipeline ✅ COMPLETE

**Budget**: 6 cycles | **Actual**: 4 cycles (Ares: 3, Apollo: 1)

Completed:
- Created `v5/queueing/` package with Buffer + Pipeline
- Removed `v5/pipelining/` directory
- Updated all imports across the codebase
- Regenerated all mocks
- All tests pass; merged to main via PR #8

## M2: Merge CI actions ✅ COMPLETE

**Budget**: 3 cycles | **Actual**: 2 cycles (Ares: 1, Apollo: 1)

Completed:
- Merged `akita_compile`, `lint`, and `akita_unit_test` into single `akita_build_lint_test` job
- Updated downstream jobs (`noc_acceptance_test`, `mem_acceptance_test`) to depend on merged job
- Merged to main via PR #9

## Project Complete ✅

All milestones achieved:
- `v5/queueing/` package exists with Buffer and Pipeline types
- No remaining imports of `v5/pipelining` in the codebase
- `v5/pipelining/` directory removed
- CI workflow has single merged job for compile+lint+test
- All PRs merged to main

## Lessons Learned

- M1 came in under budget (4 cycles vs 6). Team worked efficiently.
- M2 also came in under budget (2 cycles vs 3).
- Marcus (high model) handled both complex tasks well in single focused cycles.
- Total project: 6 cycles used across both milestones.
