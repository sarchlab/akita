# DRAM Modeling Roadmap — Parity with DRAMSim3 & Ramulator2

**Goal.** Make Akita's `mem/dram` model support the union of features offered by
**DRAMSim3** (umd-memsys, v1.0.0 / frozen 2021) and **Ramulator 2.0**
(CMU-SAFARI, active `main`), and **continuously validate** Akita's behavior
against both reference simulators.

This document is the plan of record. It is intentionally phased and
dependency-ordered: each phase produces something testable, and later phases
build on the architecture introduced in earlier ones.

> Status legend: ☐ not started · ◐ in progress · ☑ done
> Effort legend: **S** ≤1wk · **M** 1–3wk · **L** 3–6wk · **XL** >6wk

---

## 1. Where we are today

Akita's `mem/dram` is a **single-channel, timing-only** controller. It models:

- JEDEC command timing (ACT/RD/WR/PRE + auto-precharge), per-bank state machines
  (Open/Closed), and a 4-context timing table (same-bank / other-banks-in-group /
  same-rank / other-ranks): tRCD, tRP, tRAS, tCL/tCWL, tCCDL/S, tRRDL/S, tWTR,
  tRTP, tWR, tRTRS, **tFAW**.
- FR-FCFS with row-buffer-hit priority, optional read/write queue separation with
  write-drain watermarks, open/close page policy.
- Address bit-decode (row/rank/bank/bankgroup/column), an interleaving converter,
  backing `mem.Storage`, the control protocol (enable/pause/drain/reset),
  serializable checkpoint state, tracing, and basic statistics.
- Timing **formulas** cross-validated against DRAMSim3/Ramulator2 reference
  equations (66 checks).

It does **not** model: real refresh, power/energy, thermal, RowHammer/security,
configurable address mapping, multi-channel/sub-channel structure, self-refresh /
power-down, HMC, or most protocol-specific behavior. It also carries known
correctness/perf defects (see Phase 0).

A detailed current-state feature matrix lives in §3.

---

## 2. Guiding principles

1. **Pluggability first.** Ramulator2's leverage comes from config-selectable
   plugins (scheduler / row-policy / refresh / address-mapper / mitigation). We
   refactor toward the same shape *before* piling on features, so each new
   feature is an implementation of an interface rather than a fork of the
   controller.
2. **Validate every phase.** No feature is "done" until it is checked against
   DRAMSim3 and/or Ramulator2 (whichever model it). The differential harness
   (§5) is built early (Phase 0) and grows with each phase.
3. **Parity, then divergence.** Where the references disagree (e.g. write-delay
   modeling), we match one explicitly and document the deviation, as the README
   already does. We do not silently diverge.
4. **Don't regress what works.** The existing timing cross-validation
   (`timing_crossvalidation_test.go`) is the floor. Every refactor must keep it
   green.
5. **Keep the simulator fast.** This is a system-level simulator; per-cycle hot
   paths must not allocate or scan. Data-structure fixes (Phase 0) precede the
   features that would amplify the cost.

---

## 3. Target feature matrix (union of both references)

| Capability | DRAMSim3 | Ramulator2 | Akita now | Target phase |
|---|---|---|---|---|
| DDR3/4 timing | ✓ | ✓ | ✓ | — |
| DDR5 | — | ✓ | preset only | P4 |
| LPDDR / LPDDR3 / LPDDR4 | ✓ | — | enum only | P4 |
| LPDDR5 | — | ✓ | enum only | P4 |
| GDDR5 / GDDR5X | ✓ | — | enum only | P4 |
| GDDR6 | ✓ | ✓ | preset | P4 |
| HBM / HBM2 | ✓ | ✓ | HBM2/3 presets | P4 |
| HBM3 | — | ✓ | preset | P4 |
| HMC (links/vaults/xbar) | ✓ | — | enum only | P4 |
| Real refresh commands | ✓ | ✓ | ✗ (fake stall) | P2 |
| Per-bank refresh (REFpb/REFsb) | ✓ | — | ✗ | P2 |
| Rank-staggered REFab | ✓ | ✓ | ✗ | P2 |
| RFM / Directed-RFM | — | ✓ | ✗ | P2 |
| Self-refresh (SREF) | ✓ | — | ✗ | P2 |
| Power-down (PD) | stub | — | ✗ | P2 (opt) |
| Configurable address mapping | ✓ (12-field) | ✓ (named + XOR + RIT) | ✗ (1 fixed) | P3 |
| FR-FCFS scheduling | ✓ | ✓ | ✓ | — (→plugin P1) |
| Alt schedulers (BLISS, etc.) | — | ✓ | ✗ | P7 |
| Open / close page | ✓ | ✓ | ✓ | — (→plugin P1) |
| Close-after-N-accesses | — | ✓ | ✗ | P3 |
| PER_BANK / PER_RANK queues | ✓ | n/a | per-rank | P1 |
| Multi-channel / sub-channel | ✓ | ✓ | ✗ (aliases) | P1 |
| Power / energy (IDD/VDD) | ✓ | DDR4/5 | ✗ | P5 |
| Thermal model | ✓ | — | ✗ | P6 |
| RowHammer mitigations (×11) | — | ✓ | ✗ | P7 |
| PRAC + Alert-Back-Off | — | ✓ | ✗ | P7 |
| Plugin/registry architecture | — | ✓ | ✗ | P1 |
| Validated vs vendor RTL | ✓ | — | ✗ | P8 |
| Differential vs DRAMSim3/Ramulator | n/a | n/a | formula-only | P0/P8 |

---

## 4. Phased roadmap

### Dependency graph

```
P0 (fixes + harness)
   └─ P1 (pluggable arch + channels)
        ├─ P2 (refresh + low-power) ──┐
        ├─ P3 (address mapping)       │
        ├─ P4 (protocol breadth) ─────┤
        │                             ▼
        └─ P5 (power/energy) ── P6 (thermal)
                 │
                 └─ P7 (rowhammer/security)   [also needs P2 for RFM]
P8 (continuous differential validation) — spun up in P0, hardened throughout
```

---

### Phase 0 — Correctness fixes + validation harness *(prerequisite)* — **M** — ◐

Fix the defects that make further work unsafe, and stand up the differential
test infrastructure *first* so every later phase has an oracle.

**Deliverables**

- ☑ **Fix the open-page panic.** Decoupled *bank occupancy* from *data-return
  latency*. Previously a Read set the bank busy (`HasCurrentCmd`/`CycleLeft`) for
  `readDelay = tRL + burstCycle`, but the next same-bank command was gated by the
  (much smaller) timing table, so `startCommand` was called on a still-busy bank →
  `panic("previous cmd is not completed")`. Fixed: `bankState` no longer tracks a
  current command; next-command eligibility is driven solely by the timing table
  (`CyclesToCmdAvailable`) + state machine + tFAW, and data/response readiness now
  lives on `State.PendingCompletions` (a per-command timeline). Reproduced and
  regression-tested end-to-end in `p0_regression_test.go` (back-to-back same-row,
  row-conflict, 8-deep pipelined reads), plus the `mem/acceptancetests/dram`
  random stress harness now passes under `PagePolicyOpen`.
- ☑ **Hot-path data structures.** `bankState.CyclesToCmdAvailable` is now a fixed
  `[numCmdKind]int` (was `map[string]int` keyed by `fmt.Sprintf`). `findBankState`
  and `findActivateHistory` are O(1) direct indexing (added `bankFlatIndex`); the
  linear scans are gone.
- ☑ **Stable transaction references.** `subTransRef` now carries a stable `TxID`
  (transactions get an `ID`); queues, commands, and pending completions reference
  transactions by ID, so `removeTransaction` is a plain slice compaction with no
  re-indexing and no `TransIndex = -1` sentinel.
- ☑ **Channel decision.** Adopted option (a): one `dram.Comp` per channel;
  `NumChannel > 1` is rejected at build time (`channelCountMustBeOne`). First-class
  channels are deferred to P1. (Previously `location.Channel` was decoded but never
  used → silent bank aliasing.)
- ◐ **Validation harness skeleton** (see §5): `mem/dram/validation/` directory
  structure + `README.md` (plan & status) + `DEVIATIONS.md` (D1–D5) are in place.
  **Still pending:** vendoring/building the external DRAMSim3 & Ramulator2 oracles,
  the canonical-config generator, the Akita standalone trace driver, and the
  actual Tier 5 DDR4 differential run. This needs network/build access for the two
  C++ simulators and is the next P0 increment.

**Acceptance**

- ☑ Existing suite green (172 specs); new open-page back-to-back + row-conflict +
  pipelined-read + channel-guard tests pass (no panic); `go vet` and
  `golangci-lint` clean; close-page **and** open-page acceptance stress runs pass.
- ☐ Tier 5 DDR4 differential runs in CI and reports command counts / latency /
  bandwidth vs both references within agreed tolerance (§5.4). *(Blocked on oracle
  vendoring — see the ◐ deliverable above.)*

---

### Phase 1 — Pluggable controller architecture + channels — **L** — ◐

Refactor the baked-in controller into config-selectable components, mirroring
Ramulator2's interface/implementation/factory pattern, using Akita's
builder+Spec+middleware idiom.

> Increment status: **P1.1 (scaffolding + extract defaults) — done.** P1.2
> (`PER_BANK` queues) and P1.3 (channel model) — not started. Channel direction
> decided: keep one-per-channel (Option A); `AddrMapper` returns a full
> `location` so first-class channels remain possible without an interface change.

**Interfaces** (sketch — names illustrative)

```go
// Chooses the next command to put on the command bus.
type Scheduler interface {
    Pick(spec *Spec, st *State, t *dramTiming) *commandState
}
// Decides Read vs ReadPrecharge / when to auto-precharge.
type RowPolicy interface {
    CommandFor(spec *Spec, st *State, ref subTransRef) *commandState
    OnAccess(bs *bankState) // e.g. close-after-N
}
// Decides when/what refresh to inject.
type RefreshManager interface {
    Tick(spec *Spec, st *State, t *dramTiming) (cmd *commandState, stall bool)
}
// physical address -> location.
type AddrMapper interface {
    Map(spec *Spec, addr uint64) location
}
// Observes/affects every issued command (counters, tracing, RowHammer, energy).
type CommandHook interface {
    OnIssue(spec *Spec, st *State, cmd *commandState, now uint64)
}
```

**Deliverables**

- ☑ A registry keyed by config string (e.g. `spec.Scheduler = "FRFCFS"`), plus
  builder overrides (`WithScheduler`, `WithRowPolicy`, `WithRefreshManager`,
  `WithAddrMapper`, `WithPlugin`). See `plugins.go`.
- ☑ Reimplement today's behavior as the default plugins: `FRFCFS` scheduler,
  `open`/`close` row policies, `fakestall` refresh, the `default` address
  mapper, plus a no-op `null` command hook. **No behavior change** — the
  defaults delegate to the existing functions and the full pre-existing suite
  passes unmodified.
- ☐ Command-queue structure option (`PER_RANK` default, add `PER_BANK`). *(P1.2)*
- ☑ Resolve the channel model: enforced one-per-channel retained (Option A);
  the `AddrMapper` interface returns a full `location` for forward-compat. First-
  class channels remain a possible future P1.3 if needed.

**Acceptance**

- ☑ All P0 tests unchanged; new `plugins_test.go` covers registry selection,
  builder overrides, the unknown-key panic, and the hook path. *(Tier 5
  differential still blocked on oracle vendoring, as in P0.)*
- ☑ A no-op "null" plugin (and a counting hook) prove the hook path works
  without altering results.

---

### Phase 2 — Real refresh & low-power states — **L**

Replace the global `tRFC` stall with real refresh commands flowing through the
bank state machine, and implement the refresh schemes of both references.

**Deliverables**

- Refresh as real commands (`cmdKindRefresh` / `cmdKindRefreshBank`) issued by a
  `RefreshManager`, gated by the (currently dead) refresh timing-table entries in
  `generateTiming()`. **Refresh closes affected rows** (fixes the row-buffer-hit
  over-count).
- DRAMSim3 parity: `RankLevelSimultaneous`, `RankLevelStaggered` (default),
  `BankLevelStaggered` (REFpb) — selectable via `spec.RefreshPolicy`. Honor
  `tREFI`, `tREFIb`, `tRFC`, `tRFCb`.
- Ramulator2 parity: **RFM / Directed-RFM** device commands + an `RFMManager`
  plugin (DDR5/LPDDR5/GDDR6/HBM3).
- Self-refresh (SREF enter/exit), per-rank, idle-threshold-gated
  (`enable_self_refresh`, `sref_threshold`) — DRAMSim3 parity. Power-down (PD) is
  optional (DRAMSim3 stubs it too).

**Acceptance**

- Idle-then-burst traces: REF command count and refresh stall cycles match
  DRAMSim3 (each policy) and Ramulator2 (REFab) within tolerance.
- Self-refresh entry/exit cycle counts match DRAMSim3 for an idle workload.

---

### Phase 3 — Configurable address mapping — **M**

**Deliverables**

- Permutation-string mapper (DRAMSim3's 12-char `{ch,ra,bg,ba,ro,co}` form) and
  named schemes (`RoBaRaCoCh`, `ChRaBaRoCo`) — Ramulator2 parity.
- XOR-permutation mapper (`MOP4CLXOR`-style) and optionally RIT (row indirection
  table).
- Keep the existing interleaving converter for multi-controller setups.

**Acceptance**

- For identical configs, Akita's decoded `location` matches DRAMSim3 and
  Ramulator2 for a swept address set (exact match).

---

### Phase 4 — Protocol breadth — **L/XL**

**Deliverables**

- Complete, parameter-checked presets for every protocol both references support:
  DDR3/4/5, LPDDR/3/4/5, GDDR5/5X/6, HBM/2/3. (Presets are cheap; behavior is
  not.)
- Protocol-specific behavior currently absent: DDR5 two-banks-per-bank-group &
  same-bank refresh; LPDDR per-bank refresh / no-DLL specifics; GDDR burst
  structure; HBM pseudo-channels.
- **HMC** (DRAMSim3 parity): packet/link/vault/crossbar model. This is a distinct
  architecture — implement as a sibling sub-model (e.g. `mem/dram/hmc` or a
  dedicated component) rather than overloading the JEDEC controller.

**Acceptance**

- Tier 1 formula cross-validation extended to every protocol the references model.
- Tier 5 differential passes per protocol (those each reference actually ships).

---

### Phase 5 — Power / energy model — **M**

**Deliverables**

- IDD/VDD parameters in `Spec` (per-protocol presets, Micron-style).
- Per-event energy accounting: ACT, RD, WR, REF (all-bank + per-bank),
  background (active-standby IDD3N, precharge-standby IDD2N), self-refresh (IDD6).
  Mirror DRAMSim3's energy increment equations.
- Reported stats: per-component energy, `total_energy`, `average_power`
  (extend `stats.go`).

**Acceptance**

- For a fixed trace + config, energy components and totals match DRAMSim3
  (always) and Ramulator2 (DDR4/5 opt-in model) within tolerance.

---

### Phase 6 — Thermal model — **L** *(depends on P5)*

DRAMSim3's headline feature; observational (no thermal→timing feedback), which
bounds scope.

**Deliverables**

- Power-map accumulation per physical grid cell (uses P5 per-command energy +
  vendor location remapping).
- Transient (explicit time-stepping) and steady-state temperature solve; 3D
  stacking / TSV parameters for HBM/HMC.
- Heatmap/CSV outputs (`final_temp`, `epoch_*`), `[thermal]`-style config knobs,
  epoch driver.

**Acceptance**

- For a known power map / config, steady-state and transient temperatures match
  DRAMSim3's thermal output within tolerance (DRAMSim3 itself is validated
  qualitatively vs ANSYS FEM — we match DRAMSim3, not silicon).

---

### Phase 7 — RowHammer / read-disturbance & security — **XL** *(depends on P1, P2)*

Ramulator2's defining capability. Build on the P1 `CommandHook` and P2 refresh/RFM.

**Deliverables**

- Per-row / per-bank activation counters and an activation trace hook.
- Mitigation plugins (Ramulator2 set): PARA, Graphene, Hydra, TWiCe(-Ideal),
  BlockHammer, RRS, AQUA, Oracle, CounterBasedTRR.
- **PRAC** (DDR5 Per-Row Activation Counting) with the Alert-Back-Off state
  machine (`NORMAL/PRE_RECOVERY/RECOVERY/DELAY`), plus turnkey config.
- Alternative schedulers that ship alongside these (BLISS, BlockHammer scheduler,
  PRAC scheduler).
- *Out of scope (neither reference models it): ECC/RAS.* Note explicitly.

**Acceptance**

- For known hammer patterns, per-mitigation trigger counts / preventive-refresh
  counts / activation traces match Ramulator2 within tolerance.

---

### Phase 8 — Continuous differential validation — **M** *(ongoing)*

Harden the §5 harness into a maintained, version-pinned CI gate covering all the
above. This phase is "make it permanent," not "invent it" (the harness is born in
P0).

---

## 5. Validation strategy (against DRAMSim3 & Ramulator2)

The second goal — *validate against both simulators* — is a first-class workstream,
not an afterthought. There are two complementary techniques.

### 5.1 Formula cross-validation (already exists; extend)

Independently recompute each timing relationship from JEDEC parameters using the
published DRAMSim3/Ramulator2 equations and assert equality with Akita's
`generateTiming()`. This is `timing_crossvalidation_test.go` today (Tier 1, 66
checks, 3 protocols). **Extend to every protocol** as P4 lands. Cheap, fast,
no external dependency — keep it as the inner CI loop.

### 5.2 Trace-driven differential co-simulation (new — the core of P0/P8)

Feed the **same** request trace and the **same** configuration to all three
simulators and compare outputs.

**Reference oracles.** Vendor DRAMSim3 and Ramulator2 as pinned external builds
under `mem/dram/validation/` (git submodule or fetch-script + pinned commit;
record exact commit hashes). Drive them via their standalone trace frontends:
- DRAMSim3: `dramsim3main <ini> -t <trace>`; build with `-DCMD_TRACE=1` for
  command-level comparison and `-DTHERMAL=1` for P6.
- Ramulator2: `ReadWriteTrace` / `LoadStoreTrace` frontend via `ramulator2 -c
  <yaml>`.
- Akita: a new standalone trace driver (built in P0) that consumes the same trace
  format and runs the `dram.Comp` to completion.

**Config alignment (the hard part).** Maintain a single canonical JEDEC parameter
set per protocol and **generate** all three configs from it, so comparisons are
apples-to-apples:

```
validation/configs/ddr4_2400.canonical.yaml
   ├─ gen → DDR4_*.ini      (DRAMSim3)
   ├─ gen → ddr4.yaml       (Ramulator2)
   └─ gen → Spec (Go)       (Akita)
```

This forces identical: protocol & all timing params, geometry
(channel/rank/bg/bank/row/col), bus width / burst length, **address mapping**,
**scheduling policy**, **page policy**, **queue sizes/structure**, and **refresh
policy**. A divergence in any of these invalidates the comparison, so the
generator is the single source of truth.

**Trace corpus.**
- *Synthetic, targeted:* single-request-per-scenario (closed/open/conflict);
  streaming same-row (row-buffer-hit BW); strided; random; write-heavy
  (write-drain); idle-then-burst (refresh & self-refresh); hammer patterns
  (P7); bank-parallel (tRRD/tFAW).
- *Realistic:* traces captured from Akita GPU/system runs, plus standard public
  traces, to exercise mixed locality at scale.

### 5.3 Metrics compared

| Metric | Source | Match expectation |
|---|---|---|
| Command counts (ACT/PRE/RD/WR/REF/REFb) | all three | **exact** where scheduling is deterministic |
| Single-request latency | all three | **exact** (matches formula) |
| Avg / P50 / P99 read & write latency | all three | within tolerance |
| Sustained bandwidth | all three | within tolerance |
| Row-buffer hit rate | all three | within tolerance |
| Energy components + total / avg power | DRAMSim3, Ramulator2(DDR4/5) | within tolerance |
| Temperatures (steady + transient) | DRAMSim3 (`-DTHERMAL`) | within tolerance |
| Mitigation triggers / preventive refreshes | Ramulator2 | within tolerance |

### 5.4 Tolerances & divergence policy

- **Exact match** required for: command counts and single-request latency on
  deterministic scenarios; address decode.
- **Bounded match** for aggregate metrics: a documented percentage band (start
  generous, e.g. ≤5–10% on latency/BW, and tighten per protocol as we converge).
  Differences arise legitimately from arbitration tie-breaks, queue-drain order,
  and refresh phase — these are expected and bounded, not bugs.
- **Documented deviations:** where the references themselves disagree (e.g.
  `writeDelay = readDelay` vs `tWL + burstCycle`, already noted in the README),
  pick and document the reference we match. Each accepted divergence gets a note
  in `validation/DEVIATIONS.md` with rationale.

### 5.5 Test tiers (overall)

1. **Tier 1** — timing-formula cross-validation *(exists)*
2. **Tier 2** — single-request latency *(exists)*
3. **Tier 3** — microbenchmark behavior: tCCD/tRRD/tFAW *(exists)*
4. **Tier 4** — bandwidth sanity *(exists)*
5. **Tier 5** — **full-trace differential vs DRAMSim3 & Ramulator2** *(new, P0+)*
6. **Tier 6** — **per-feature differential**: refresh (P2), power (P5), thermal
   (P6), RowHammer (P7)

Tiers 1–4 run in-process every CI build. Tiers 5–6 require the external oracles;
run them in a dedicated (possibly nightly) CI job keyed on the pinned reference
commits.

---

## 6. Proposed directory layout

```
mem/dram/
  (existing core .go files — refactored into plugins during P1)
  scheduler/         # FRFCFS + future schedulers (P1, P7)
  rowpolicy/         # open / close / close-after-N (P1, P3)
  refresh/           # REFab / REFpb / RFM / self-refresh (P2)
  addrmap/           # permutation / named / XOR / RIT (P3)
  power/             # IDD/VDD energy model (P5)
  thermal/           # power maps + solver (P6)
  security/          # rowhammer mitigations + PRAC (P7)
  hmc/               # HMC sub-model (P4)
  validation/
    configs/         # canonical params + generators -> ini/yaml/Spec
    traces/          # synthetic + captured corpus
    oracles/         # pinned DRAMSim3 / Ramulator2 build scripts + commits
    DEVIATIONS.md    # accepted, documented divergences
    diff/            # metric comparison tooling
```

---

## 7. Risks & open questions

- **External-oracle maintenance.** DRAMSim3 is frozen at 2021 (no DDR5/HBM3) and
  Ramulator2 lacks LPDDR4/GDDR5/HMC — so for several protocols **only one**
  reference exists, and for some emerging features (PRAC) only Ramulator2. The
  matrix in §3 records which oracle applies where; some protocols can only be
  formula-validated (Tier 1), not differentially co-simulated.
- **Determinism gap.** Cycle-accurate simulators differ in arbitration tie-breaks;
  exact aggregate match is unrealistic. Tolerances (§5.4) are essential and must
  be justified, not hand-waved.
- **Config-generation fidelity.** The whole differential method rests on the
  canonical-config generator producing truly equivalent configs. This is the
  single highest-leverage piece of infra to get right (and to test in isolation).
- **Scope of HMC and thermal.** Both are large, self-contained subsystems; treat
  each as its own mini-project with its own go/no-go.
- **Performance budget.** Refresh, energy, and thermal add per-cycle work; the
  Phase 0 data-structure fixes are a hard prerequisite, and we should track
  simulated-cycles/sec as a regression metric.

---

## 8. Suggested sequencing summary

1. **P0** — fix the panic + hot paths + channel decision; stand up the
   differential harness (DDR4). *Nothing is trustworthy until this lands.*
2. **P1** — pluggable architecture + channel model (no behavior change).
3. **P2 / P3 / P4** — real refresh & low-power, configurable address mapping,
   protocol breadth (parallelizable once P1 lands).
4. **P5 → P6** — energy, then thermal (thermal depends on energy).
5. **P7** — RowHammer/security (depends on P1 hooks + P2 refresh/RFM).
6. **P8** — make differential validation a permanent CI gate.
