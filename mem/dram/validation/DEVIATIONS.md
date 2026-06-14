# Accepted / Documented Deviations

Where Akita's `mem/dram` model intentionally differs from DRAMSim3 and/or
Ramulator2, or where the two references disagree and we pick one, the divergence
is recorded here with its rationale (ROADMAP §5.4 "documented deviations"). A
deviation listed here is a *known, accepted* gap — not a silent one.

| # | Topic | Akita behavior | Reference behavior | Status |
|---|---|---|---|---|
| D1 | Write latency | `WriteDelay = TRL + BurstCycle` | DRAMSim3 uses `tWL + BurstCycle` | Accepted; pre-existing, asserted in `timing_crossvalidation_test.go` |
| D2 | Refresh | Global `tRFC` stall every `tREFI`; does **not** issue real refresh commands or close rows | Real per-rank/per-bank refresh commands through the bank state machine | Known gap — fixed in roadmap **P2** |
| D3 | Close-page read/write data latency | Sub-transaction completes `tRP` cycles after a `ReadPrecharge`/`WritePrecharge` is issued (`buildCmdCycles`), not `readDelay`/`writeDelay` | Data returns `tRL/tWL + burst` after the column command; precharge follows | Known gap — preserved as-is in P0 to avoid regressing behavior; revisit once the differential harness can validate the corrected timing |
| D4 | Channels | One `dram.Comp` models exactly one channel; `NumChannel > 1` is rejected at build time | Both references model multiple channels internally | Intentional for now — first-class channels are roadmap **P1** |
| D5 | Address mapping | Single fixed bit-decode scheme | DRAMSim3 12-field permutation; Ramulator2 named + XOR + RIT | Known gap — configurable mapping is roadmap **P3** |

## Notes on the P0 timing model

Bank *occupancy* is **not** modeled as a single busy flag. Next-command
eligibility is driven entirely by the per-bank timing table
(`CyclesToCmdAvailable`), the open/closed state machine, and tFAW. The
data/response readiness of a read/write is tracked separately on
`State.PendingCompletions`. This decoupling is what allows correctly pipelined
column commands (it also fixed the open-page "previous cmd is not completed"
panic). It is a *more* faithful model than the previous busy-flag approach, not a
deviation — the per-command latencies it schedules are unchanged from before
(see D3 for the one latency value still carried over as a known gap).
