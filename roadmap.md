# Akita v5 Refactor — Roadmap

## Project Goal

Create `v5` folder as the next generation of the Akita simulation engine. The key API change: ports are no longer created inside builders — instead, ports must be passed in from outside. Produce a `migration.md` and a PR for human review.

Additionally, migrate CI to use the "Marin group" self-hosted runners (replacing `ubuntu-latest` and `Github-Large-Runners`).

## Success Criteria (from spec)

- v5 folder exists with v4 code as starting point
- Port creation API refactored: ports passed in from outside, not created in builders
- `migration.md` written in the v5 directory explaining the refactored API
- PR created in akita-dev repo (not merged)

---

## Milestones

### M1: Setup v5 scaffold and CI migration ✅ COMPLETE (3 cycles)
- Copied v4 code into `v5/` folder
- Updated module path to `github.com/sarchlab/akita/v5`
- Migrated CI to self-hosted runners
- `go build ./...` passes

### M2: Refactor port creation API ✅ COMPLETE (5 cycles)
- Added `SetComponent(comp Component)` to Port interface
- Refactored all 21 non-test builder files: ports passed in via `WithXxxPort()` methods
- Updated all callers (tests, acceptance tests)
- `go build ./...`, `go vet ./...`, `go test ./...` all pass (38 packages)
- Branch: `ares/m1-v5-scaffold`

### M3: Write migration.md and create PR ✅ COMPLETE (2 cycles)
- `v5/migration.md` exists with comprehensive content (port API, SetComponent, CI migration, V5 philosophy, queueingv5, CLI changes)
- PR #1 opened from `ares/m1-v5-scaffold` → `main` (not merged, awaiting human review)
- `go build ./...` and `go vet ./...` both pass clean

### M4: Fix CI — add tool setup steps to workflow ⬜ IN PROGRESS
- CI is failing because the self-hosted runner doesn't have `go`, `npm`, or `python3` in PATH
- Need to add `actions/setup-go`, `actions/setup-node`, `actions/setup-python` actions to all jobs
- Reference the original akita repo's CI for the correct patterns
- Estimated: 2 cycles

---

## Lessons Learned

- M2 budget was 4 cycles but took 5. Large refactoring across 21+ files benefits from more generous budgets.
- Splitting work across multiple workers (simple/medium/complex builders) worked well for parallel-like execution.
- `go vet` issues (unkeyed fields in dram) caught late — should run vet earlier in the process.

---

## Cycle Log

| Cycle | Manager | What Happened |
|-------|---------|---------------|
| 1 | Athena | Initial research and roadmap creation |
| 2-4 | Ares/Apollo | M1 completed and verified |
| 5 | Athena | M2 defined, issue #4 created |
| 6-8 | Ares | M2 port refactoring (3 worker cycles) |
| 9 | Apollo | M2 verified — PASS |
| 10 | Athena | M2 complete, M3 defined |
| 11-12 | Ares | M3 completed (migration.md + PR) |
| 13 | Apollo | M3 verified — PASS |
| 14 | Athena | All milestones complete, project done |
