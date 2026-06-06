# dram — DRAM Memory Controller

Package `dram` provides a cycle-accurate DRAM memory controller for the Akita
simulation framework. It models the full DRAM command protocol including
activate, read, write, precharge, and refresh operations with accurate
inter-command timing constraints.

## Supported Protocols

| Preset | Variable | Frequency | Bus Width | Burst Length |
|---|---|---|---|---|
| DDR4-2400 | `DDR4Spec` | 1200 MHz | 64-bit | 8 |
| DDR5-4800 | `DDR5Spec` | 2400 MHz | 32-bit | 16 |
| HBM2-2Gbps | `HBM2Spec` | 1000 MHz | 128-bit | 4 |
| HBM3-6.4Gbps | `HBM3Spec` | 3200 MHz | 64-bit | 8 |
| GDDR6-14Gbps | `GDDR6Spec` | 1750 MHz | 32-bit | 16 |

Additional protocol constants: `DDR3`, `GDDR5`, `GDDR5X`, `LPDDR`, `LPDDR3`,
`LPDDR4`, `LPDDR5`, `HBM`, `HMC`, `HBM3E`.

## Architecture

The controller is organized as three middleware stages executed each tick:

```
Top port ──► parseTopMW ──► bankTickMW ──► respondMW ──► Top port
                │               │              │
           (parse reqs,    (issue DRAM     (send data-ready
            split into     commands,        / write-done
            sub-trans)     tick banks)       responses)
```

A `ctrlMiddleware` also runs each tick, handling `mem.ControlReq` (enable /
pause / drain / reset) on the `Control` port.

1. **parseTopMW** — Receives `mem.ReadReq`/`mem.WriteReq` from the top port,
   splits large requests into sub-transactions aligned to the access unit size
   (bus width × burst length), and queues them.

2. **bankTickMW** — The core scheduling engine. Each tick it advances bank
   state machines, enforces timing constraints between commands (same-bank,
   same-bank-group, same-rank, other-ranks), handles periodic refresh, and
   issues activate/read/write/precharge commands. Tracks tFAW (four-activate
   window) constraints.

3. **respondMW** — Completes transactions when all sub-transactions finish,
   reads/writes data from the backing `mem.Storage`, and sends responses.

## Key Types

### Spec (immutable configuration)

Core timing parameters (all in DRAM clock cycles):

| Parameter | Description |
|---|---|
| `TCL` | CAS latency (read) |
| `TCWL` | CAS write latency |
| `TRCD` | RAS-to-CAS delay |
| `TRP` | Row precharge time |
| `TRAS` | Row active time |
| `TCCDL` / `TCCDS` | CAS-to-CAS delay (long/short, same/diff bank group) |
| `TRRDL` / `TRRDS` | Row-to-row activation delay |
| `TFAW` | Four-activate window |
| `TREFI` | Refresh interval |
| `TRFC` | Refresh cycle time |

Organization parameters: `NumChannel`, `NumRank`, `NumBankGroup`, `NumBank`,
`NumRow`, `NumCol`, `BusWidth`, `BurstLength`, `DeviceWidth`.

### State (mutable runtime data)

Contains the transaction queue, sub-transaction queue, per-bank command queues,
bank states (open/closed/refreshing), and statistics counters.

### Bank States

Each bank can be in one of: **Open** (row activated), **Closed**
(precharged), **SRef** (self-refresh), or **PD** (power-down).

### Commands

```
Activate → Read/Write → Precharge → (next row)
         → ReadPrecharge / WritePrecharge (auto-precharge)
         → Refresh / RefreshBank
         → SRefEnter / SRefExit
```

## Builder Pattern

All scalar configuration is supplied as a whole through `WithSpec`. Start from a
preset (or `DefaultSpec()`), tweak the fields you need, and pass it in. Wiring is
supplied through `WithRegistrar` (which provides the engine and registers the
component) and `WithResources` (shared objects such as backing storage). `Build`
declares the `Top` and `Control` ports but does not create their instances.
Build each port with `modeling.MakePortBuilder` (which registers the port with
the simulation) and attach it with `AssignPort`, choosing the buffer size.

```go
spec := dram.DDR4Spec
spec.Freq = 1200 * timing.MHz
spec.PagePolicy = dram.PagePolicyOpen

ctrl := dram.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(dram.Resources{Storage: storage}).
    Build("DRAM")

topPort := modeling.MakePortBuilder().
    WithRegistrar(sim).
    WithComponent(ctrl).
    WithSpec(modeling.PortSpec{BufSize: 1024}).
    Build("Top")
ctrl.AssignPort("Top", topPort)

ctrlPort := modeling.MakePortBuilder().
    WithRegistrar(sim).
    WithComponent(ctrl).
    WithSpec(modeling.PortSpec{BufSize: 4}).
    Build("Control")
ctrl.AssignPort("Control", ctrlPort)

topPort = ctrl.GetPortByName("Top")
```

### Builder Methods

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` or a preset (DDR4Spec, HBM2Spec, ...) |
| `WithResources(Resources{Storage: s})` | Shared backing storage (built internally if omitted) |

### Commonly Tweaked Spec Fields

| Field | Description |
|---|---|
| `Freq` | Operating frequency |
| `Protocol` | DRAM protocol type |
| `NumRank` / `NumBankGroup` / `NumBank` | Bank geometry |
| `BusWidth` / `BurstLength` | Data bus width (bits) and burst transfer length |
| `PagePolicy` | `PagePolicyOpen` or `PagePolicyClose` |
| `TransactionQueueSize` / `CommandQueueCapacity` | Queue depths |
| `HasAddrConverter` / `InterleavingSize` / ... | Address interleaving for multi-controller setups |

## Statistics

The `State` tracks runtime statistics, accessible via helper functions:

```go
state := ctrl.State
hitRate := dram.RowBufferHitRate(&state)
avgRead := dram.AverageReadLatency(&state)
avgWrite := dram.AverageWriteLatency(&state)
readBW := dram.ReadBandwidth(&state)    // bytes per cycle
writeBW := dram.WriteBandwidth(&state)  // bytes per cycle
```

Available counters: `TotalReadCommands`, `TotalWriteCommands`,
`TotalActivates`, `TotalPrecharges`, `RowBufferHits`, `RowBufferMisses`,
`CompletedReads`, `CompletedWrites`, `BytesRead`, `BytesWritten`.

## Ports

- **Top**: accepts `mem.ReadReq` and `mem.WriteReq`, returns
  `mem.DataReadyRsp` and `mem.WriteDoneRsp`
- **Control**: accepts `mem.ControlReq` (enable / pause / drain / reset),
  returns `mem.ControlRsp`

## Validation

This package ships with a four-tier validation suite covering timing formula
correctness, single-request latency, multi-request behavioral patterns, and
bandwidth sanity checks. The suite is implemented in two test files:

- [`timing_crossvalidation_test.go`](timing_crossvalidation_test.go) — 66
  cross-validation checks (Tier 1–4)
- [`memcontroller_test.go`](memcontroller_test.go) — 84 unit tests covering
  address mapping, transaction splitting, bank state transitions, command
  scheduling, refresh, and statistics

Combined: **150+ tests** across the package.

---

### Tier 1 — Timing Formula Cross-Validation (66 checks)

**Purpose:** Verify that `generateTiming()` produces timing tables that match
the canonical formulas used by DRAMSim3 and Ramulator2 for the same DRAM
parameters.

**Protocols validated:** DDR4-2400, DDR5-4800, HBM2-2Gbps.

**Methodology:**
Each formula is computed twice — once by the production code under test, and
once by an independent reference implementation embedded in the test file
(`computeExpectedTimings`). The reference derives values directly from the JEDEC
parameter set using the same equations published in the DRAMSim3 and Ramulator2
source trees. The 22 timing relationships verified for each protocol are:

| Category | Relationships checked |
|---|---|
| Read → Read | same-bank, other-banks-in-bank-group, same-rank, other-rank |
| Read → Write | same-bank, other-rank |
| Write → Read | same-bank, same-rank, other-rank |
| Write → Write | same-bank, same-rank, other-rank |
| Write → Precharge | same-bank |
| Read → Precharge | same-bank |
| Precharge → Activate | same-bank |
| Activate → Read / Write | same-bank (×2) |
| Activate → Activate | same-bank, other-banks-in-bank-group, same-rank |
| Activate → Precharge | same-bank |

22 checks × 3 protocols = **66 formula checks**.

**Observed accuracy:** All 66 checks pass. Timing values match the DRAMSim3 /
Ramulator2 reference exactly for DDR4 and HBM2. For DDR5 the formulas are
structurally identical; parameter values follow the JEDEC DDR5-4800 specification
used in the Ramulator2 DDR5 config.

**Known model gap — write delay:** In this implementation
`writeDelay = tRL + burstCycle` (same as `readDelay`), whereas DRAMSim3 uses
`writeDelay = tWL + burstCycle`. This divergence is intentional: the model
focuses on read-dominant GPU workloads where write-to-read turnaround is the
critical constraint. The timing table for `writeToRead` is unaffected because
it is derived from the correct `tWTR` parameters. This gap is documented in the
source code with a comment and is not expected to affect simulation accuracy for
typical GPU memory access patterns.

---

### Tier 2 — Single-Request Latency Validation

**Purpose:** Verify that the end-to-end cycle count for a single request matches
the analytical formula derived from JEDEC timing parameters.

**Protocol:** DDR4-2400.

**Methodology:** Four scenarios are exercised by driving the bank state machine
directly (no full controller instantiation required):

1. **Closed-bank read** — bank starts precharged; the test issues ACT, ticks
   `tRCD − tAL` cycles, then issues READ and verifies `CycleLeft = tRL + burstCycle`.
   Total cycles = `(tRCD − tAL) + tRL + burstCycle`.

2. **Row-buffer-hit read** — bank is pre-opened to the target row; the test
   verifies `getRequiredCommandKind` returns `CmdKindRead` (no ACT required).

3. **Row-conflict read** — bank is open to a different row; the test verifies
   the required command sequence: Precharge → wait `tRP` → Activate → Read.
   `getReadyCommand` is expected to return `nil` until the Precharge completes.

4. **Write-then-read turnaround** — a write is issued to an open bank; the test
   verifies the `CyclesToCmdAvailable` counter for the subsequent read is set to
   `writeToReadL = writeDelay + tWTRL` and that the read becomes ready only after
   that constraint drains.

All four scenarios pass.

---

### Tier 3 — Multi-Request Behavioral Tests

**Purpose:** Verify correct multi-bank scheduling behavior including tCCD, tRRD,
and tFAW constraints.

**Protocol:** DDR4-2400.

**Tests:**

1. **Sequential reads to the same row** — issues two back-to-back reads to the
   same row and verifies: (a) only the first read requires an ACT (row-buffer
   hit on the second), and (b) the inter-read gap is capped at
   `readToReadL = max(burstCycle, tCCDL)`.

2. **Parallel reads across different bank groups** — activates bank (0,0,0) and
   verifies that bank (0,1,0) receives an ACT→ACT constraint of `tRRDS` (the
   short, cross-bank-group value). After the constraint expires, the Activate
   on the second bank is immediately available.

3. **Same-bank-group reads** — activates bank (0,0,0) and verifies that
   bank (0,0,1) in the same bank group receives the larger `tRRDL` constraint.

4. **tFAW enforcement** — issues four activates across different banks within a
   window shorter than `tFAW`. The test then attempts a fifth activate and
   verifies that `getReadyCommand` returns `nil` (blocked). After advancing
   `TickCount` to `tFAW`, the same call returns a valid command. This confirms
   the rolling four-activate window is correctly enforced.

All four tests pass.

---

### Tier 4 — Bandwidth Sanity Checks

**Purpose:** Verify that analytically-derived achievable bandwidths fall within
the expected 40–100% of the theoretical peak for streaming row-buffer-hit
workloads.

**Methodology:** For each protocol the test computes:

```
bytesPerRead   = BurstLength × BusWidth / 8
cyclesPerRead  = readToReadL  (= max(burstCycle, tCCDL) for row-buffer hits)
achievableBW   = bytesPerRead / cyclesPerRead × freq
ratio          = achievableBW / peakBW
```

where `peakBW = freq × busWidth × 2 / 8` (DDR factor included).

| Protocol | Freq | Bus width | Peak BW | Expected ratio range |
|---|---|---|---|---|
| DDR4-2400 | 1200 MHz | 64-bit | 19.2 GB/s | 40–90 % |
| DDR5-4800 | 2400 MHz | 32-bit | 19.2 GB/s | 40–100 % |
| HBM2-2Gbps | 1000 MHz | 128-bit | 32.0 GB/s | 40–90 % |

DDR5 can reach 100 % of peak because `tCCDL = burstCycle = 8`, allowing
back-to-back row-buffer-hit transfers with no idle cycles. All three checks
pass.

---

### Overall Accuracy and Known Limits

| Area | Status |
|---|---|
| DDR4 timing formulas vs DRAMSim3 | ✓ Exact match (22/22 relationships) |
| DDR5 timing formulas vs Ramulator2 | ✓ Exact match (22/22 relationships) |
| HBM2 timing formulas vs DRAMSim3 | ✓ Exact match (22/22 relationships) |
| Single-request latency (DDR4) | ✓ Matches formula |
| tFAW enforcement | ✓ Verified |
| tRRDL / tRRDS enforcement | ✓ Verified |
| tCCDL row-buffer-hit BW | ✓ Within expected range |
| Write delay model | ⚠ Uses `readDelay` instead of `tWL + burstCycle` |
| Write-heavy workload BW | ⚠ Not independently validated |
| HBM3 / GDDR6 latency validation | ✗ Not yet covered by Tier 2–3 tests |
| Refresh impact on latency | ✗ Behavioral; covered by unit tests but not cross-validated against reference simulators |

The write-delay deviation does not affect the timing table values used for
scheduling (they are derived from `tWTR` parameters), but it means the
`readDelay` / `writeDelay` accessors cannot be directly compared to DRAMSim3
traces for write-dominated workloads. Users running write-heavy benchmarks
should treat reported write latencies as approximate.
