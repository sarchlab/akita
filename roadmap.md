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

### M1: Setup v5 scaffold and CI migration ✅ COMPLETE
**Budget:** 3 cycles | **Actual:** 3 cycles (Ares 2, Apollo 1)

- ✅ Copied all v4 code from the akita public repo into `v5/` folder
- ✅ Set up go module for v5 (`github.com/sarchlab/akita/v5`)
- ✅ Migrated GitHub Actions CI to self-hosted runners
- ✅ `go build ./...` passes
- Branch: `ares/m1-v5-scaffold`

### M2: Refactor port creation API [ACTIVE]
**Budget:** 6 cycles

The core refactoring: change all builders so that ports are passed in from the outside rather than created internally.

**Scope:**
- 21 non-test files contain `sim.NewPort()` calls inside builders/constructors
- Key packages: sim, mem/cache (writeback, writethrough, writeevict, writearound), mem/vm (tlb, mmu, gmmu, mmuCache, addresstranslator), mem/idealmemcontroller, mem/dram, mem/simplebankedmemory, mem/datamover, noc, examples (ping, tickingping)
- All callers (tests, acceptance tests) must be updated to create ports externally and pass them in
- `go build ./...` and `go test ./...` must pass after refactoring

**Pattern:**
```go
// v4 (old): ports created inside builder
func (b Builder) Build(name string) *Comp {
    c := &Comp{}
    c.topPort = sim.NewPort(c, bufSize, bufSize, name+".TopPort")
    c.AddPort("Top", c.topPort)
    return c
}

// v5 (new): ports passed in from outside
func (b Builder) WithTopPort(port sim.Port) Builder {
    b.topPort = port
    return b
}
func (b Builder) Build(name string) *Comp {
    c := &Comp{}
    c.topPort = b.topPort
    c.AddPort("Top", c.topPort)
    return c
}

// Caller:
topPort := sim.NewPort(comp, bufSize, bufSize, "MyComp.TopPort")
comp := MakeBuilder().WithTopPort(topPort).Build("MyComp")
```

### M3: Write migration.md and create PR [PLANNED]
**Budget:** 2 cycles

- Write clear `v5/migration.md` documenting the API change with before/after examples
- Open PR in akita-dev with all changes
- PR description summarizes what changed and why

---

## Lessons Learned

- M1 went smoothly in 3 cycles. Worker skills were appropriate.
- Leo (high-tier) was effective for the code migration task.
- Max (mid-tier) was appropriate for CI setup.

---

## Cycle Log

| Cycle | Manager | What Happened |
|-------|---------|---------------|
| 1 | Athena | Initial research and roadmap creation. Dispatched Ares for M1. |
| 2 | Ares | Leo copied v4→v5, Max added CI. Both complete. |
| 3 | Ares | Verified and claimed M1 complete. |
| 4 | Apollo | Verified M1. All checks pass. |
| 5 | Athena | Evaluated M1 completion. Preparing M2 (port refactoring). |
