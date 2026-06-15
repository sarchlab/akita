# Reference oracles: DRAMSim3 & Ramulator2

This directory builds the two external reference simulators at **pinned commits**
(`COMMITS.txt`) and documents how we drive them and read their stats. The
committed reference data in `../data/` was produced by exactly these builds.

## Build

```bash
# Host toolchain (git, cmake>=3.14, C++20 compiler):
./build_oracles.sh                 # -> ./.oracles/{DRAMsim3,ramulator2}/build/...

# Or hermetic (any host with Docker):
docker build -t dram-oracles .
```

Both build cleanly with cmake + g++ 13 (verified). Ramulator2 fetches a few
build-time deps (argparse/yaml-cpp/spdlog) during cmake, so the build needs
network access.

## How each oracle is driven

Both consume a **physical address** trace and apply their own address mapping,
so we feed the same logical access pattern to all three simulators (see
`../traces/`).

### DRAMSim3 — `dramsim3main <config.ini> -c <cycles> -t <trace> -o <outdir>`

- Trace line format: `0x<ADDR_HEX> <READ|WRITE>  <arrival_cycle>` (memory-clock
  domain). Parser: `src/common.cc operator>>(Transaction)`.
- Config: `.ini` with `[dram_structure] [timing] [power] [system]` sections.
  Relevant `[system]` knobs we align: `address_mapping`, `row_buf_policy`
  (`OPEN_PAGE`/`CLOSE_PAGE`), `queue_structure`, `cmd_queue_size`,
  `trans_queue_size`, `channels`.
- Stats: writes `dramsim3.json` (+ `.txt`) to `-o` dir, keyed by channel.
  Fields we read: `num_act_cmds`, `num_pre_cmds`, `num_read_cmds`,
  `num_write_cmds`, `num_reads_done`, `num_writes_done`, `num_read_row_hits`,
  `num_ref_cmds`, `average_read_latency`, `num_cycles`, `average_bandwidth`.

### Ramulator2 — `ramulator2 -f <config.yaml>`

- Frontend `LoadStoreTrace`, trace line format: `LD|ST <addr>` (`0x` hex or
  decimal). Parser: `src/frontend/impl/memory_trace/loadstore_trace.cpp`. The
  run ends when every line has been issued once and memory drains.
- Config: YAML (`Frontend` / `MemorySystem.DRAM` / `Controller` / `AddrMapper`).
  We align `Controller.RowPolicy` (`ClosedRowPolicy cap:1` == pure close-page),
  `Scheduler: FRFCFS`, `AddrMapper`, and the DRAM org/timing.
- Stats: a YAML dump to stdout — `total_num_read_requests`,
  `avg_read_latency_*`, `read_row_hits_*`, `memory_system_cycles`.
- Per-command counts via the **`CommandCounter`** controller plugin
  (`commands_to_count: [ACT, PRE, RD, WR, RDA, WRA, REFab]`, `path: <csv>`),
  which writes `cmd, count` lines.

## Cross-simulator metric normalization

The three simulators use different command conventions, so the diff compares a
**normalized** schema, not raw fields:

| Normalized metric | DRAMSim3 | Ramulator2 | Akita |
|---|---|---|---|
| `activates` | `num_act_cmds` | `ACT` (CommandCounter) | `TotalActivates` |
| `reads` (RD+RDA) | `num_read_cmds` | `RD`+`RDA` | `TotalReadCommands` |
| `writes` (WR+WRA) | `num_write_cmds` | `WR`+`WRA` | `TotalWriteCommands` |
| `avg_read_latency` | `average_read_latency` | `avg_read_latency_0` | driver-measured |

**Precharge counting is intentionally NOT compared directly:** DRAMSim3 folds
auto-precharge into `num_pre_cmds`, while Ramulator2 and Akita treat `RDA`/`WRA`
as a single auto-precharge column command with no separate `PRE`. In pure
close-page every activate is eventually precharged, so `activates` is the
faithful cross-sim quantity. This divergence is recorded in `../DEVIATIONS.md`.

`activates`/`reads`/`writes` are **config-determined** for the close-page
deterministic scenarios (independent of exact timing and address mapping), so
they are compared **exactly**. `avg_read_latency` depends on timing alignment
and is compared with a tolerance band (see `../diff/`).
