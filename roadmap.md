# Roadmap

## Project Goal

Evolve Akita V5: redefine component model, implement save/load, make messages plain structs, and port all first-party components to the new model with fully serializable State.

## Completed Milestones

### M1: Create `modeling` package with Component struct ✅ — Budget: 6, Used: 5
### M2: Refactor `idealmemcontroller` to use modeling package ✅ — Budget: 6, Used: 4
### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8, Used: 6
### M4: Fix CI lint failures ✅ — Budget: 3, Used: 2
### M5: Redesign Messages as Plain Structs ✅ — Budget: 8, Used: 6

Replaced `sim.Msg` interface with concrete `Msg` struct. All 31 message types converted to payload structs. PR #14 merged.

### M6: Port All First-Party Components ✅

#### M6.1–M6.4: Ported all 16 tick-driven components structurally ✅ — Budget: 16, Used: 8

### M7: Move Mutable Runtime Data into State Structs — IN PROGRESS

#### M7.1: Simple components (no queueing/cache deps) ✅ — Budget: 6, Used: 4
Populated State for: mmuCache, mmu, gmmu, addresstranslator, datamover, endpoint.

#### M7.2: Components with queueing deps ✅ — Budget: 6, Used: 4
Added queueing snapshot functions (SnapshotPipeline/RestorePipeline/SnapshotBuffer/RestoreBuffer). Populated State for: simplebankedmemory, switches, TLB. PR #20 merged.

#### M7.3: DRAM State Population ✅ — Budget: 6, Used: 2
Populated State for DRAM with full pointer-graph flattening (527-line state.go). PR #21 merged.

#### M7.4: Cache State Population (writearound, writeevict, writethrough) ✅ — Budget: 6, Used: 2
Shared Directory/MSHR serialization helpers in v5/mem/cache/state_helpers.go. State population for 3 near-identical cache variants. PR #22 merged.

#### M7.5: Writeback Cache State Population — DEFERRED
Analysis complete (Diana's report). ~565 lines estimated. **Deferred pending Msg redesign decision** — building this with MsgRef would require rewriting after the redesign.

## M8: Msg-as-Interface Redesign — IN PROGRESS

Per human direction (#93), `sim.Msg` becomes an interface. Design by Iris (workspace/iris/msg_as_interface_design.md), risk analysis by Diana (workspace/diana/msg-as-interface-redteam.md).

**Design decisions:**
- `sim.Msg` interface with single `Meta() *MsgMeta` method
- `MsgMeta` base struct embeds all routing fields, gains `Meta()` pointer method (auto-satisfies interface when embedded)
- 30 payload types → 30 concrete message structs (e.g., `mem.ReadReq` embeds `sim.MsgMeta`)
- Builders return concrete types; Ports accept `sim.Msg` interface
- State files store concrete types directly (no MsgRef)
- `Info interface{}` tagged `json:"-"` for now (isolated problem, separate fix later)

### M8.1: Foundation — Msg interface + GenericMsg rename + Port/Connection updates — CURRENT
**Scope:** sim package only (+ directconnection, wiring, tracing, analysis). ~20 files.
1. Rename current `Msg` struct → `GenericMsg` in sim package
2. Add `Msg` interface (`Meta() *MsgMeta`)
3. Move `RspTo` from `GenericMsg` into `MsgMeta`; add `MsgMeta.Meta()` and `MsgMeta.IsRsp()`
4. Update `Port` interface: `*Msg` → `Msg` (interface) for all 6 message methods
5. Update `portBuffer`: `[]*Msg` → `[]Msg`
6. Update `defaultPort`, `directconnection`, `wiring/port`, `wiring/wire`
7. Update `tracing/api.go`, `analysis/port_analyzer.go`
8. Update all `msgMustBeValid` helpers
9. `GenericMsg` satisfies `Msg` via embedding → all existing code still compiles

### M8.2: Convert Protocol Packages — AFTER M8.1
Convert 30 payload types to concrete message types:
- `mem/mem/protocol.go` (6 types), `mem/cache/protocol.go` (4), `mem/vm/protocol.go` (4)
- `mem/vm/tlb/tlbprotocol.go` (4), `mem/vm/mmuCache/mmuCacheprotocol.go` (4)
- `mem/datamover/protocol.go` (1), `noc/messaging/flit.go` (1)
- `examples/ping` (2), `examples/tickingping` (2), `noc/acceptance` (1), `noc/standalone` (1)
- Remove `GenericMsg`, `MsgPayload[T]`, `TryMsgPayload[T]`

### M8.3: Update All Components — AFTER M8.2
Update all component files to use concrete types:
- Replace `msg.Payload.(type)` switches with `msg.(type)` switches
- Replace `MsgPayload[T](msg)` with direct field access on concrete type
- Update all transaction structs to hold concrete types
- ~40+ component files

### M8.4: Update State Files — AFTER M8.3
- Remove `MsgRef`, `MsgRefFromMsg`, `MsgFromRef`, `state_helpers.go`
- Update all state.go files to store concrete types directly
- Complete writeback cache state (was M7.5)

### M9: Comp Wrapper Elimination — AFTER M8
Human issue #61.

## Known Bugs
- Payload loss in ALL msgRef implementations → nil Payload after save/load → panics (will be fixed by M8)
- 6 identical msgRef definitions across packages (will be eliminated by M8)

## Previously Completed Goals
1. **Component Model** — `modeling.Component[S,T]` with Spec/State/Ports/Middlewares
2. **Save/Load** — `simulation.Save()`/`Load()` with deterministic acceptance test
3. **Messages as Plain Structs** — `sim.Msg` is concrete struct with typed payloads
4. **Port All Components** — 16 tick-driven components structurally ported (State populated for 14/15; remaining: writeback)
5. **CI Passes** — All checks green on main

## Lessons Learned
- M1-M3 completed in 15 implementation cycles (budgeted 20).
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's/Diana's detailed analysis before milestones prevents scope misestimation.
- Multi-worker parallel approach is the winning pattern for mechanical refactoring.
- M6 sub-milestones consistently completed in 2 cycles each (budgeted 4).
- Research for M7 revealed that the queueing/cache serialization problem is deeper than expected — need phased approach.
- The `*sim.Msg` pointer problem is universal across all components — the idealmemcontroller decomposition pattern is the proven solution.
- queueing snapshot functions (Approach A) proved pragmatic — standalone functions avoiding interface changes + no mock updates.
- M7.1–M7.4 each completed well under budget — consistent track record. Total: 12 cycles used vs 24 budgeted.
- Total project: ~41 implementation cycles across 78 orchestrator cycles. Overhead from planning/verification is worthwhile (catches real bugs like the Payload loss).
- **NEW**: When a core design issue surfaces (like the Payload problem), it's better to address it before building more on the broken foundation. M7.5 deferred to avoid rework.
