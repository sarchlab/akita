# Accepted / Documented Deviations

Where Akita's `mem/dram` model intentionally differs from DRAMSim3 and/or
Ramulator2, or where the two references disagree and we pick one, the divergence
is recorded here with its rationale (ROADMAP §5.4 "documented deviations"). A
deviation listed here is a *known, accepted* gap — not a silent one.

| # | Topic | Akita behavior | Reference behavior | Status |
|---|---|---|---|---|
| D1 | Write latency | `WriteDelay = TRL + BurstCycle` | DRAMSim3 uses `tWL + BurstCycle` | Accepted; pre-existing, asserted in `timing_crossvalidation_test.go` |
| D2 | Refresh | Global `tRFC` stall every `tREFI`; does **not** issue real refresh commands or close rows | Real per-rank/per-bank refresh commands through the bank state machine | Known gap — fixed in roadmap **P2** |
| D3 | Close-page read/write data latency | Sub-transaction completes `readDelay`/`writeDelay` cycles after the column command, including the `ReadPrecharge`/`WritePrecharge` auto-precharge variants (`buildCmdCycles`); the trailing precharge is enforced by the bank timing table | Data returns `tRL/tWL + burst` after the column command; precharge follows | **Resolved in P0** — completion timeline now uses the data-return latency for the auto-precharge variants instead of `tRP` |
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
deviation — the per-command latencies it schedules are the faithful data-return
latencies, including for the close-page auto-precharge variants (see D3, resolved
in P0).
