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

### M8.1: Foundation — Msg interface + GenericMsg rename + Port/Connection updates ✅ — Budget: 8, Used: 3
Renamed `Msg` → `GenericMsg`, added `Msg` interface with `Meta() *MsgMeta`, updated Port/Buffer/Connection/tracing/analysis. PR #23 merged.

### M8.2: Convert all payloads to concrete message types + remove GenericMsg ✅ — Budget: 8, Used: 9
**Scope:** Convert all 30 payload types to concrete message structs embedding `sim.MsgMeta`, update all ~40 component files and ~50 test files, remove `GenericMsg`/`MsgPayload[T]`/`TryMsgPayload[T]`. Also remove all `msgRef` definitions and simplify state serialization.

Per human direction (#101): simplicity is the top priority. GenericMsg must go. Each package defines concrete, serializable message types. No runtime type casting, no `Payload any`.

**Conversion pattern for each payload type:**
```go
// BEFORE: ReadReqPayload + builder returns *sim.GenericMsg
type ReadReqPayload struct { Address uint64; ... }
func (b ReadReqBuilder) Build() *sim.GenericMsg { ... }

// AFTER: ReadReq embedding MsgMeta, builder returns *ReadReq
type ReadReq struct { sim.MsgMeta; Address uint64; ... }
func (b ReadReqBuilder) Build() *ReadReq { ... }
```

**Component update pattern:**
```go
// BEFORE
msg := port.RetrieveIncoming().(*sim.GenericMsg)
payload := sim.MsgPayload[mem.ReadReqPayload](msg)
payload.Address

// AFTER
msg := port.RetrieveIncoming()
switch req := msg.(type) {
case *mem.ReadReq:
    req.Address
}
```

**Protocol files (6):** mem/mem/protocol.go, mem/cache/protocol.go, mem/vm/protocol.go, mem/vm/tlb/tlbprotocol.go, mem/vm/mmuCache/mmuCacheprotocol.go, mem/datamover/protocol.go
**Other msg files (4):** noc/messaging/flit.go, examples/ping, examples/tickingping, noc/acceptance+standalone
**Component files (~32):** all consumers of MsgPayload or *GenericMsg
**Test/mock files (~50):** corresponding test updates
**Cleanup:** remove GenericMsg, MsgPayload[T], TryMsgPayload[T] from sim/msg.go; remove all msgRef types; simplify state.go files

**Interfaces to update:**
- `AccessReqPayload` / `AccessRspPayload` in mem/mem/protocol.go → replace with `AccessReq` / `AccessRsp` interfaces on the new concrete types
- `tracing/api.go` functions taking `*sim.GenericMsg` → update to take `sim.Msg` (handle tracing differently)

### M8.3: Remove message builders + msgRef cleanup — NEXT
Per human direction (#109): messages are pure data, so builder pattern is unnecessary boilerplate. Remove all ~22 message builder types across 7 protocol files. Replace with direct struct literal construction. Also remove all `msgRef` types (9 files) — store concrete message types directly in state.

**Scope:**
1. Remove all message builder structs and their methods from protocol files
2. Update all ~30+ call sites to use direct struct literal construction
3. Add a simple `sim.InitMsg(meta *MsgMeta, src, dst RemotePort)` helper for ID generation + common setup
4. Remove all `msgRef`/`MsgRef` types and conversion functions from state files
5. Update state serialization to store concrete message types directly
6. Update all tests

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
- **M8.1 completed in 3 cycles (budgeted 8).** Multi-worker approach continues to work well for mechanical changes.
- **Human feedback in #101 reinforces simplicity-first**: GenericMsg must go, fewer concepts = better API. Merged M8.2-M8.4 scope into a single aggressive milestone.
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
