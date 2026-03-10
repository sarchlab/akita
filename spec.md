# Project Spec

## What to Build

1. **Create a new `v5/queueing/` package** and move the following into it:
   - `Buffer` interface and `bufferImpl` (currently in `v5/sim/buffer.go`)
   - `Pipeline` interface and `pipelineImpl` (currently in `v5/pipelining/`)
   - All associated types, builders, hook positions, and tests

2. **Update all references** across the codebase to import from `github.com/sarchlab/akita/v5/queueing` instead of `sim.Buffer` or the `pipelining` package.

3. **Merge CI actions**: Combine the `akita_compile`, `lint`, and `akita_unit_test` jobs in `.github/workflows/akita_test.yml` into a single CI job.

## Constraints

- The old `v5/pipelining/` package should be removed after migration.
- `sim.Buffer` and `sim.NewBuffer` should be removed from `v5/sim/` and placed in `v5/queueing/`.
- Backward compatibility aliases in `v5/sim/` are acceptable if needed, but the canonical location must be `v5/queueing/`.
- All existing tests must continue to pass after the refactoring.
- The merged CI job should perform compile, lint, and unit test in sequence.

## Success Criteria

- `v5/queueing/` package exists with Buffer and Pipeline types.
- No remaining imports of `v5/pipelining` anywhere in the codebase.
- `sim.Buffer` references are updated to `queueing.Buffer` (or aliased).
- CI workflow has a single merged job for compile+lint+test instead of three separate jobs.
- All CI checks pass (compile, lint, unit tests, acceptance tests).

