# Roadmap

## M1: Create `v5/queueing/` package with Buffer and Pipeline ✅ COMPLETE

**Budget**: 6 cycles | **Actual**: 4 cycles (Ares: 3, Apollo: 1)

Completed:
- Created `v5/queueing/` package with Buffer + Pipeline
- Removed `v5/pipelining/` directory
- Updated all imports across the codebase
- Regenerated all mocks
- All tests pass; merged to main via PR #8

## M2: Merge CI actions (budget: 3 cycles)

**Goal**: Merge `akita_compile`, `lint`, and `akita_unit_test` jobs into a single job in `.github/workflows/akita_test.yml`.

The three jobs to merge:
1. `akita_compile` — sets up Go, installs mockgen, generates mocks, builds
2. `lint` — sets up Go, generates mocks, runs golangci-lint
3. `akita_unit_test` — sets up Go, installs mockgen+ginkgo, generates mocks, runs ginkgo tests

The merged job should:
- Run compile, lint, and unit test in sequence in a single job
- Keep the same tools (Go 1.24.7, mockgen, ginkgo, golangci-lint)
- Other jobs (`akitartm_compile`, `daisen_compile`, `noc_acceptance_test`, `mem_acceptance_test`) should remain unchanged but update their `needs` dependencies if they referenced the old jobs

**Status**: Not started → Next milestone for Ares

## Lessons Learned

- M1 came in under budget (4 cycles vs 6). Team worked efficiently.
- Marcus (high model) handled the complex refactoring well in a single focused task.
- Breaking the fix into a second focused issue (#3 for monitoring test fix) was effective.
