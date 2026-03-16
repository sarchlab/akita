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
TopPort ──► parseTopMW ──► bankTickMW ──► respondMW ──► TopPort
               │               │              │
          (parse reqs,    (issue DRAM     (send data-ready
           split into     commands,        / write-done
           sub-trans)     tick banks)       responses)
```

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

```go
ctrl := dram.MakeBuilder().
    WithEngine(engine).
    WithSpec(dram.DDR4Spec).
    WithFreq(1200 * sim.MHz).
    WithTopPort(topPort).
    Build("DRAM")
```

### Common Builder Options

| Method | Description |
|---|---|
| `WithEngine(e)` | Event scheduler (required) |
| `WithSpec(s)` | Use a preset spec (DDR4Spec, HBM2Spec, etc.) |
| `WithFreq(f)` | Operating frequency |
| `WithTopPort(p)` | Port for read/write requests |
| `WithProtocol(p)` | DRAM protocol type |
| `WithNumRank(n)` | Number of ranks |
| `WithNumBankGroup(n)` | Number of bank groups |
| `WithNumBank(n)` | Banks per bank group |
| `WithBusWidth(n)` | Data bus width in bits |
| `WithBurstLength(n)` | Burst transfer length |
| `WithGlobalStorage(s)` | Shared backing storage |
| `WithPagePolicy(p)` | `PagePolicyOpen` or `PagePolicyClose` |
| `WithTransactionQueueSize(n)` | Transaction buffer depth |
| `WithInterleavingAddrConversion(...)` | Address interleaving for multi-controller setups |

## Statistics

The `State` tracks runtime statistics, accessible via helper functions:

```go
state := ctrl.GetState()
hitRate := dram.RowBufferHitRate(&state)
avgRead := dram.AverageReadLatency(&state)
avgWrite := dram.AverageWriteLatency(&state)
readBW := dram.ReadBandwidth(&state)    // bytes per cycle
writeBW := dram.WriteBandwidth(&state)  // bytes per cycle
```

Available counters: `TotalReadCommands`, `TotalWriteCommands`,
`TotalActivates`, `TotalPrecharges`, `RowBufferHits`, `RowBufferMisses`,
`CompletedReads`, `CompletedWrites`, `BytesRead`, `BytesWritten`.

## Protocol

- **Top port**: accepts `mem.ReadReq` and `mem.WriteReq`, returns
  `mem.DataReadyRsp` and `mem.WriteDoneRsp`
