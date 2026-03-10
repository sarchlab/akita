# Akita v5 Refactor — Roadmap

## Project Goal

Create `v5` folder as the next generation of the Akita simulation engine. The key API change: ports are no longer created inside builders — instead, ports must be passed in from outside. Produce a `migration.md` and a PR for human review.

Additionally, migrate CI to use the "Marin group" self-hosted runners (replacing `ubuntu-latest` and `Github-Large-Runners`).

## Success Criteria (from spec)

- v5 folder exists with v4 code as starting point
- Port creation API refactored: ports passed in from outside, not created in builders
- `migration.md` written in the v5 directory explaining the refactored API
- PR created and merged in akita-dev repo
- CI passes green on all jobs

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

### M4: Fix CI — add tool setup steps to workflow ✅ COMPLETE (2 cycles)
- Added `actions/setup-go`, `actions/setup-node` to all CI jobs
- Used system `python3` instead of `actions/setup-python` (Fedora arm64 runner)
- Fixed mock generation, funlen lint, and type errors in acceptance tests
- All 7 CI jobs pass green (run 22881461969)

---

## Lessons Learned

- M2 budget was 4 cycles but took 5. Large refactoring across 21+ files benefits from more generous budgets.
- Splitting work across multiple workers (simple/medium/complex builders) worked well for parallel-like execution.
- `go vet` issues (unkeyed fields in dram) caught late — should run vet earlier in the process.
- Self-hosted runners (Fedora arm64) don't support `actions/setup-python` — use system `python3` instead.
- CI should be validated early after any infrastructure change, not left as an afterthought.

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
| 15-18 | Ares | M4 CI fix (tool setup, mock gen, lint, python) |
| 19 | Athena | M4 verified complete, project done |
| 20 | Athena | Human requested PR merge (issue #14). Resolved merge conflict. Defining M5. |
| 21-23 | Ares/Apollo | M5 completed — PR merged, verified |
| 24 | Athena | Final review — project complete |

### M5: Merge PR and claim completion ✅ COMPLETE (2 cycles)
- Resolved merge conflicts (roadmap.md)
- CI passed green (all 14 checks)
- PR #1 merged into main (commit f096838)
- Project complete

---

## Final Status: ✅ PROJECT COMPLETE

All success criteria met:
- v5/ folder created with v4 code, module path updated to v5
- Port creation API fully refactored (21 builders, SetComponent pattern)
- migration.md written with comprehensive documentation
- PR #1 merged after CI passed (all 7 jobs × 2 runs = 14 checks green)
- CI migrated to self-hosted Marin runners
- Total cycles used: 24
