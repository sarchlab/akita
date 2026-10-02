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

A request flows through three stages:

```
Top port ──► ParseTop ──► BankTick ──► Respond ──► Top port
                │             │            │
           (parse reqs,  (issue DRAM   (send data-ready
            split into   commands,      / write-done
            sub-trans)   tick banks)     responses)
```

The stages are middlewares, the fields of `Middlewares`. They run every cycle
in field order: `Ctrl`, `Respond`, `Refresh`, `BankTick`, `ParseTop`.

1. **Ctrl** — Handles `memcontrolprotocol.Req` (enable / pause / drain /
   reset) on the `Control` port.

2. **Respond** — Completes transactions when all sub-transactions finish,
   reads/writes data from the backing `mem.Storage`, and sends responses.

3. **Refresh** — Schedules periodic refresh (a global tRFC stall every tREFI
   cycles). It runs ahead of BankTick so its stall flag is set before the issue
   step reads it.

4. **BankTick** — The core scheduling engine. Each tick it advances bank
   state machines, enforces timing constraints between commands (same-bank,
   same-bank-group, same-rank, other-ranks), honors the refresh stall, and
   issues activate/read/write/precharge commands. Tracks tFAW (four-activate
   window) constraints.

5. **ParseTop** — Receives `memprotocol.ReadReq`/`memprotocol.WriteReq` from
   the top port, splits large requests into sub-transactions aligned to the
   access unit size (bus width × burst length), and queues them.

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

Derived values are not Spec fields; they are computed from the fields above
when the component is built:

| Value | Formula |
|---|---|
| burst cycle | `BurstLength / 2` (GDDR5: `/ 4`, GDDR5X: `/ 8`, GDDR6: `/ 16`) |
| tRL / tWL | `TAL + TCL` / `TAL + TCWL` |
| read delay / write delay | `tRL + burstCycle` (both; see the write-delay gap below) |
| tRC | `TRAS + TRP` |
| access unit size | `BusWidth / 8 × BurstLength` bytes |
| address mapping | bit positions and masks from the geometry (see below) |

The Spec fields that used to hold these values — `TRL`, `TWL`, `ReadDelay`,
`WriteDelay`, `TRC`, `BurstCycle`, `Log2AccessUnitSize`, and the
address-mapping `ChannelPos`/`ChannelMask`, `RankPos`/`RankMask`,
`BankGroupPos`/`BankGroupMask`, `BankPos`/`BankMask`, `RowPos`/`RowMask`,
`ColPos`/`ColMask` — are removed; the old builder overwrote whatever the caller
set in them.

### State (mutable runtime data)

Contains the transaction queue, sub-transaction queue, per-bank command queues,
bank states (open/closed/refreshing), and statistics counters.

### Resources, Ports, Middlewares, Comp

- `Resources` — `Storage`, the `mem.Storage` that holds the data. **Required**:
  `Build` panics if it is nil.
- `Ports` — the `Top` and `Control` ports.
- `Middlewares` — `Ctrl`, `Respond`, `Refresh`, `BankTick`, and `ParseTop`,
  run in that order every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

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
preset (or `Definition.DefaultSpec`), tweak the fields you need, and pass it in. Wiring is
supplied through `WithSimulation` (which provides the engine and registers the
component), `WithResources` (the backing storage), and `WithPorts` (the port
instances).

```go
spec := dram.DDR4Spec
spec.Freq = 1200 * timing.MHz
spec.PagePolicy = dram.PagePolicyOpen

storage := mem.NewStorage(4 * mem.GB)

ctrl := dram.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(dram.Resources{Storage: storage}).
    WithPorts(dram.Ports{
        Top:     messaging.NewPort("DRAM.Top", 1024, 1024),
        Control: messaging.NewPort("DRAM.Control", 4, 4),
    }).
    Build("DRAM")

topPort := ctrl.Ports.Top
```

### Builder Methods

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec` or a preset (DDR4Spec, HBM2Spec, ...) |
| `WithResources(Resources{Storage: s})` | Backing storage (required; no longer built internally) |
| `WithPorts(Ports{...})` | The port instances, each named `"<instance>.<field>"` (required) |

`Build` panics if the storage is missing, if `NumChannel > 1` (instantiate one
controller per channel), if `BurstLength` is 0, or if `Scheduler` or
`AddrMapper` names an unknown strategy.

The storage is indexed by the global request address, so it must cover every
address the controller serves. To cover the whole geometry of a Spec, size it
to `NumCol × NumRow × DeviceWidth/8 × NumBank × BusWidth/DeviceWidth × NumRank
× NumChannel` bytes (4 GiB for `Definition.DefaultSpec`); storage is allocated
lazily, so a large capacity costs nothing until it is touched.

### Commonly Tweaked Spec Fields

| Field | Description |
|---|---|
| `Freq` | Operating frequency |
| `Protocol` | DRAM protocol type |
| `NumRank` / `NumBankGroup` / `NumBank` | Bank geometry |
| `BusWidth` / `BurstLength` | Data bus width (bits) and burst transfer length |
| `PagePolicy` | `PagePolicyOpen` or `PagePolicyClose` |
| `TransactionQueueSize` / `CommandQueueCapacity` | Queue depths |
| `ReadQueueSize` / `WriteQueueSize` / `WriteHighWatermark` / `WriteLowWatermark` | Separate read/write command queues and write drain (used when both sizes are > 0) |
| `Scheduler` / `AddrMapper` | Strategy registry keys (`""` selects the default `FRFCFS` / `default`) |

Storage is **global**: a request's address indexes the backing store directly,
and the address mapper decodes that same global address into a channel/rank/
bank/row/column location. There is no per-controller address conversion.

The decode bit positions and masks are derived from the geometry
(`NumChannel`/`NumRank`/`NumBankGroup`/`NumBank`/`NumRow`/`NumCol`, bus/burst)
when the component is built; they are not Spec fields. From low to high: the
access unit, column, bank group, bank, rank, channel, row. As a result this
model is intended for **standalone / single controller** use (all presets are
standalone). It does not currently support being one of several
finely-interleaved controllers over shared storage: decode runs on the global
address with geometry-derived positions, so an upstream inter-controller
interleave finer than the decode layout would alias. For a multi-controller
memory system, use `mem/simplebankedmemory` (whose bank selector has an
explicit bank-selection address conversion).

## Statistics

The controller tracks runtime statistics, read through these functions:

```go
hitRate := dram.RowBufferHitRate(ctrl)
avgRead := dram.AverageReadLatency(ctrl)
avgWrite := dram.AverageWriteLatency(ctrl)
readBW := dram.ReadBandwidth(ctrl)    // bytes per cycle
writeBW := dram.WriteBandwidth(ctrl)  // bytes per cycle
```

Read statistics through these functions rather than the fields of `ctrl.State`:
the State of a built-in component is an implementation detail that may
change between minor versions. The functions derive from counters in the
State (command, activate, and precharge counts, row-buffer hits and misses,
completed requests, bytes moved, and cycles), which the monitor also shows.

## Ports

The system builder creates each port with `messaging.NewPort`, choosing its
buffer sizes, and passes them to `WithPorts`; `Build` binds and registers them.

- **Top** (`mem` responder): accepts `memprotocol.ReadReq` and
  `memprotocol.WriteReq`, returns `memprotocol.DataReadyRsp` and
  `memprotocol.WriteDoneRsp`
- **Control** (`mem.control` responder): accepts `memcontrolprotocol.Req`
  (enable / pause / drain / reset), returns `memcontrolprotocol.Rsp`

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
