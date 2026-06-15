# DRAM Differential Validation Harness

This directory hosts the trace-driven differential validation of Akita's
`mem/dram` model against **DRAMSim3** and **Ramulator2**, as described in
[`../ROADMAP.md`](../ROADMAP.md) §5.

> **Status: skeleton.** The directory structure and the documented-deviation
> ledger exist. The external reference oracles are **not yet vendored**, so the
> full-trace differential runs (roadmap Tier 5) are **not yet wired up**. This is
> the next increment of Phase 0 — it requires network access to fetch and build
> the two C++ simulators, which is a separate, build-heavy task.

## Layout

```
validation/
  configs/   canonical JEDEC parameter sets + generators -> {Akita Spec, .ini, .yaml}
  traces/    synthetic + captured request-trace corpus
  oracles/   pinned DRAMSim3 / Ramulator2 fetch+build scripts + recorded commits
  diff/      metric-comparison tooling
  DEVIATIONS.md   accepted, documented divergences from the references
```

## How the differential method will work

1. **One canonical config per protocol** lives in `configs/`. A generator emits
   all three forms (Akita `Spec`, DRAMSim3 `.ini`, Ramulator2 `.yaml`) from that
   single source so the comparison is apples-to-apples. This generator is the
   highest-leverage piece of infrastructure to get right (roadmap §7).
2. **The same request trace** (`traces/`) is fed to all three simulators:
   - Akita via a standalone trace driver (to be added) that consumes the trace
     and runs `dram.Comp` to completion.
   - DRAMSim3 via `dramsim3main <ini> -t <trace>`.
   - Ramulator2 via its trace frontend (`ramulator2 -c <yaml>`).
3. **Metrics are compared** with the tolerances in `DEVIATIONS.md` / ROADMAP §5.4
   (exact match for command counts & single-request latency on deterministic
   scenarios; bounded match for aggregate latency/BW/hit-rate).

## Oracle availability caveat

DRAMSim3 is frozen at 2021 (no DDR5/HBM3) and Ramulator2 lacks
LPDDR4/GDDR5/HMC. For several protocols only **one** reference exists, and a few
features (e.g. PRAC) only Ramulator2 models. The feature matrix in ROADMAP §3
records which oracle applies where; some protocols can only be formula-validated
(Tier 1, see `../timing_crossvalidation_test.go`), not differentially
co-simulated.

## What already validates Akita today (no external oracle needed)

- **Tier 1 — timing-formula cross-validation** and **Tier 4 — bandwidth sanity**
  in `../timing_crossvalidation_test.go` (independent re-derivation of the
  DRAMSim3/Ramulator2 timing equations).
- **Tier 2/3 — single-request & multi-request behavior** in the same file plus
  `../timing_validation_test.go`.
- **End-to-end correctness** via the `mem/acceptancetests/dram` random
  read/write stress harness (data-verified), and the open-page regression tests
  in `../p0_regression_test.go`.
