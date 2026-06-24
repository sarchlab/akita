# DRAM Differential Validation Harness

This directory hosts the trace-driven differential validation of Akita's
`mem/dram` model against **DRAMSim3** and **Ramulator2**, as described in
[`../ROADMAP.md`](../ROADMAP.md) §5.

> **Status: live for close-page command counts.** Both oracles build and run at
> pinned commits (`oracles/`), and `run_oracles.py` produces the committed
> reference data in `data/reference.csv`. The Akita-side Tier-5 comparison runs
> in CI as `../validation_tier5_test.go` (no C++ build needed — it reads the
> committed CSV). The first scenario family (pure close-page read/write) is
> wired end-to-end; mixed-op, open-page (locality), and latency-aligned
> comparisons are the next increments (see "Current coverage" below).

## Layout

```
validation/
  configs/   canonical JEDEC parameter sets + generators -> {Akita Spec, .ini, .yaml}
  traces/    synthetic + captured request-trace corpus
  oracles/   pinned DRAMSim3 / Ramulator2 fetch+build scripts + recorded commits
  diff/      metric-comparison tooling
  DEVIATIONS.md   accepted, documented divergences from the references
```

## Recreating the experiment

```bash
# 1. Build the pinned reference simulators (host toolchain or Docker):
oracles/build_oracles.sh                    # -> oracles/.oracles/...

# 2. Run both oracles over every scenario and (re)write the committed data:
python3 run_oracles.py                      # -> data/reference.csv, traces/scenarios.json

# 3. Check Akita against the committed reference (fast, no C++):
go test ./mem/dram/ --ginkgo.focus="Tier 5"
```

`run_oracles.py` is the single source of truth: it defines the canonical DDR4
parameters and the scenarios (dumped to `traces/scenarios.json`, which the Go
test reads so both sides drive the identical workload), emits each oracle's
config+trace, runs them, and writes `data/reference.csv`.

## Current coverage

| Tier | Scenarios | Metric | Tol. | Status |
|---|---|---|---|---|
| 5 — counts | `cp_read_64/256`, `cp_write_64/256` (close-page) | `activates`/`reads`/`writes` | exact | ✅ both oracles agree; Akita matches |
| 6 — latency (enforced) | `op_seq_64B`, `op_stride_128K` (open-page) | avg read latency vs DRAMSim3 | 15% | ✅ Akita within 8% / 0.1% |
| 6 — latency (known gap) | `op_stride_8K`, `op_stride_16K` (open-page) | avg read latency vs DRAMSim3 | 15% | ⚠️ **gap 54–63%** — see Findings |

### Findings so far

1. **Single-request latency: +1 cycle** (Akita 38 vs DRAMSim3 37). Accepted — a
   fixed offset within tolerance, likely a latency-measurement boundary.
2. **Address-mapping performance gap (KNOWN GAP, roadmap P3).** Akita has a
   single fixed address map and cannot be configured to match the references'.
   When a stride serializes to one bank (`op_seq_64B`, `op_stride_128K`) Akita
   matches DRAMSim3. When bank parallelism depends on the mapping
   (`op_stride_8K`, `op_stride_16K`) Akita's fixed map spreads accesses across
   bank groups that DRAMSim3's `rochrababgco` does not, so Akita is **54–63%
   faster** for the same nominal config. The Tier-6 suite asserts this gap is
   *currently* large; when P3 lands and it closes, the characterization spec
   fails — that is the cue to flip the scenario to `latency_check: enforced`.
3. **Row-buffer-hit-rate statistic is broken (bug, not a feature gap).**
   `RowBufferHits`/`RowBufferMisses` count every issued read as a hit and every
   activate as a miss (because by the time a read issues its bank is always
   open), so the rate is meaningless — e.g. 512 "hits" for 512 all-miss
   accesses. `RowBufferHitRate` should not be trusted or used as a metric until
   fixed. Surfaced by the open-page sweep against DRAMSim3's row-hit counts.

### Still to do

| Axis | Why deferred |
|---|---|
| Mixed read/write counts | needs the Ramulator2 drain fix (tail-subtraction is single-type) |
| Ramulator2 latency/bandwidth | its trace frontend never drains memory |
| Open-page count comparison | needs aligned per-oracle address encoders |

### Two method notes (so the numbers are trustworthy)

- **Refresh is disabled for these count scenarios** (`tREFI` pushed out of
  range). DRAMSim3 runs the full `-c` cycle budget and idles long after the
  trace drains, firing many refreshes (one boundary even adds a stray
  activate); refresh is a separate axis (P2). Akita command counts are
  refresh-independent regardless.
- **Ramulator2 does not drain memory** — `src/main.cpp` stops the moment the
  frontend has *injected* every request (the frontend source even marks this
  `TODO: FIXME`), so queued commands go uncounted. We recover exact counts by
  **tail-subtraction**: append a long type-matched drain suffix so the real ops
  fully drain, then subtract a suffix-only run (the identical trailing deficit
  cancels). This needs single-type scenarios, which is why mixed-op is deferred.

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
