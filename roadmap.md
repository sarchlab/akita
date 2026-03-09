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

### M1: Setup v5 scaffold and CI migration [PLANNED]
**Budget:** 3 cycles

- Copy all v4 code from the akita public repo into a `v5/` folder in akita-dev
- Set up the go module for v5 (`github.com/sarchlab/akita/v5`)
- Migrate GitHub Actions CI to use self-hosted runners (`runs-on: self-hosted`) instead of `ubuntu-latest` and `Github-Large-Runners`
- Get CI passing with v5 code compiling

### M2: Refactor port creation API [PLANNED]
**Budget:** 4 cycles

- Identify all builders that create ports internally
- Change the API so ports are passed in from outside (via `WithXxxPort(port sim.Port)` builder methods)
- Update all usages (examples, tests) to reflect the new pattern
- Ensure all tests pass

### M3: Write migration.md and create PR [PLANNED]
**Budget:** 2 cycles

- Write clear `v5/migration.md` documenting the API change with before/after examples
- Open PR in akita-dev with all changes
- PR description summarizes what changed and why

---

## Lessons Learned

*(none yet — project just started)*

---

## Cycle Log

| Cycle | Manager | What Happened |
|-------|---------|---------------|
| 1 | Athena | Initial research and roadmap creation. Dispatching Ares for M1. |
