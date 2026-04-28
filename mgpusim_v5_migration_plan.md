# MGPUSim V5 Migration Plan (Milestone #660)

## Purpose

This document defines a **planning-only** migration strategy for porting both `sarchlab/mgpusim` and `sarchlab/mgpusim-dev` from Akita V4 to the current Akita V5 repository.

> **Out of scope for this milestone:** no production code migration is performed here. This is sequencing and risk planning only.

## Current status and historical audit notes

- **Current Akita V5 baseline:** the module remains `github.com/sarchlab/akita/v5` (`go.mod:1`) with Go language version `1.26.0` and toolchain `go1.26.2` (`go.mod:51-53`, `TOOLCHAIN_VERSIONS.md:5-12`). The current Akita checkout passes `go test ./...`; keep that command green as the local baseline before handing a downstream migration branch to mgpusim validation.
- **Release/tag readiness:** no V5 tag is currently visible from `git ls-remote --tags origin 'v5*'` (command returns no rows), so downstream migration planning should still assume a moving branch or explicit pseudo-version until a tag/freeze point exists.
- **Historical M2 audit finding (report-only):** the M2 report branch `e2-m2-audit-migration-report-against-akita-v5-reality` was based on Akita V5 source baseline `0b80658` (`git merge-base main HEAD`) and carried a documentation-only diff (`git diff --name-only main...HEAD` returned only `mgpusim_v5_migration_plan.md`). At that time, before generated mock repair, `go test ./...` failed on missing generated mocks in packages including `mem/cache/writeback`, `mem/cache/writethroughcache`, `mem/trace`, `mem/vm/gmmu`, `mem/vm/mmu`, `mem/vm/mmuCache`, `mem/vm/tlb`, and `noc/networking/switching/switches`; representative errors included `undefined: MockPort`, `undefined: MockEngine`, `undefined: MockTimeTeller`, and `undefined: MockTable`. Treat those failures as historical audit context, not as current branch status.
- **Generated-mock status:** generated mocks and generation hooks are now present in the checkout (`run_before_merge.sh:9-18`; `mem/vm/gmmu/generate_mocks.go:1-6`; examples: `mem/cache/writeback/mock_sim_test.go:1-20`, `mem/vm/gmmu/mock_port_test.go:1-20`, `noc/networking/switching/switches/mock_sim_test.go:1-20`). Migration work should preserve these generated fixtures or run generation before validation.
- **Package reality checked:** `go list ./sim ./mem ./noc/directconnection ./queueing ./monitoring ./daisen ./simulation ./tracing ./modeling` resolves all listed packages, so these are valid Akita V5 planning targets.

---

## 1) Scope Split: `mgpusim` vs `mgpusim-dev` and Dependency Impact

### Repository roles

- **`mgpusim-dev`**: primary development surface (larger, active integration work, broad timing/driver/system builder coverage).
- **`mgpusim`**: public-facing release repository that should receive changes after `mgpusim-dev` migration stabilizes.

### Practical migration scope split

- **Phase-first target:** migrate and validate in `mgpusim-dev`.
- **Follow-up target:** backport/cherry-pick/replay stable migration commits into `mgpusim`.
- This minimizes churn in the public repo while APIs are still being adapted.

### Akita dependency hotspots (shared by both repos)

The migration impact is concentrated in subsystems that import Akita heavily:

| Subsystem area | Typical Akita V4 usage | Migration impact | Current Akita V5 evidence |
|---|---|---:|---|
| `amd/timing/cu`, `amd/timing/cp` | `sim`, `mem/*`, pipeline flow, tracing, control messages | Very high | `sim.MsgMeta` now embeds uint64 IDs and `RspTo` (`sim/msg.go:8-23`); `mem.ControlReq`/`ControlRsp` unify control commands (`mem/protocol.go:84-112`); tracing task APIs use uint64 IDs (`tracing/api.go:26-45`, `tracing/task.go:36-49`). |
| `amd/driver`, `amd/protocol` | message IDs, response matching, `MsgMeta`, VM/mem protocols | High | `sim.IDGenerator.Generate()` returns `uint64` (`sim/idgenerator.go:13-17`); `sim.MsgMeta` has `ID`, `RspTo`, `SendTaskID`, and `RecvTaskID` as `uint64` (`sim/msg.go:8-17`). |
| `amd/samples/runner` and timing configs | system wiring, monitoring/reporting/tracing, cache builders | High | `simulation.Builder` wires engines, DB tracing, and monitoring (`simulation/builder.go:64-127`); monitoring remains a V5 package wrapping Daisen rather than a pure rename (`monitoring/doc.go:1-14`, `monitoring/monitor.go:24-50`). |
| `amd/timing/mem/*`, `rdma`, `rob` | pipeline/buffer flow + message protocol updates | Medium-high | V5 queueing exposes generic `Buffer[T]` and `Pipeline[T]` value/state types (`queueing/buffer.go:15-23`, `queueing/pipeline.go:3-18`), so V4 `pipelining` code must be converted semantically, not only renamed. |

Dependency mapping highlights to carry into implementation:

| V4-style dependency | V5 planning target | Audit note |
|---|---|---|
| `akita/v4/sim` | `github.com/sarchlab/akita/v5/sim` | Valid package (`go.mod:1`; command: `go list ./sim`). Time, IDs, events, ports, and handlers all need API review (`sim/freq.go:5-24`, `sim/event.go:3-17`, `sim/port.go:29-58`). |
| `akita/v4/mem/mem` | `github.com/sarchlab/akita/v5/mem` | Valid package (`go list ./mem`); memory protocols now include unified control messages in `mem/protocol.go:84-112`. |
| `akita/v4/sim/directconnection` | `github.com/sarchlab/akita/v5/noc/directconnection` | Valid package (`go list ./noc/directconnection`); builder returns a `*Comp` wrapper over `modeling.Component` (`noc/directconnection/builder.go:28-54`). |
| `akita/v4/pipelining` | `github.com/sarchlab/akita/v5/queueing` | Valid package (`go list ./queueing`), but generic typed buffers/pipelines mean downstream pipeline state and test expectations must be rewritten (`queueing/buffer.go:15-23`, `queueing/pipeline.go:12-18`). |
| `akita/v4/monitoring` | Usually `github.com/sarchlab/akita/v5/monitoring`, plus `tracing`/`daisen` as needed | Corrected from a simple `monitoring` -> `daisen` rename. V5 still has `monitoring.NewMonitor()` (`monitoring/doc.go:7-14`, `monitoring/monitor.go:53-59`) and it wraps Daisen/replay internals (`monitoring/monitor.go:24-50`, `monitoring/monitor.go:140-176`). |
| V4 ad-hoc simulation setup | `github.com/sarchlab/akita/v5/simulation` where applicable | The V5 simulation builder owns engine/data-recorder/tracer/monitor setup (`simulation/builder.go:64-127`) and Save/Load has quiescence and SerialEngine constraints (`simulation/saveload.go:54-60`, `simulation/saveload.go:169-210`). |

---

## 2) V4 -> V5 Breaking-Change Categories

The port should be tracked by category, not by file, to reduce regressions:

1. **Import/module path updates**: the module path is `github.com/sarchlab/akita/v5` (`go.mod:1`), but imports are not purely mechanical because package boundaries and builders changed (`noc/directconnection/builder.go:28-54`, `mem/cache/writeback/builder.go:216-220`).
2. **Time and frequency type migration**: `VTimeInSec` is picoseconds as `uint64` (`sim/event.go:3-4`), `Freq` is `uint64`, and `Period()` uses integer picosecond math (`sim/freq.go:5-24`). Remove float-time assumptions except where V5 tracing storage deliberately serializes some DB fields as float (`tracing/dbtracer.go:26-54`).
3. **ID model migration**: message/task IDs and response matching use `uint64`; `IDGenerator.Generate()` returns `uint64` (`sim/idgenerator.go:13-17`), and `MsgMeta` stores `ID`, `RspTo`, `SendTaskID`, and `RecvTaskID` as `uint64` (`sim/msg.go:8-17`).
4. **Message model changes**: response linkage is through `MsgMeta.RspTo`, and `MsgMeta.IsRsp()` treats `RspTo != 0` as a response (`sim/msg.go:14-23`).
5. **Control protocol unification**: memory control flows converge on `mem.ControlReq`/`mem.ControlRsp` with commands `CmdFlush`, `CmdInvalidate`, `CmdDrain`, `CmdReset`, `CmdPause`, and `CmdEnable` (`mem/protocol.go:84-112`).
6. **Event serialization/handler identity changes**: `Event` exposes `HandlerID() string`; `EventBase` stores `HandlerID_`; engines dispatch via registered handler name (`sim/event.go:6-17`, `sim/engine.go:17-20`, `sim/serialengine.go:41-44`, `sim/serialengine.go:97-104`).
7. **Component model changes**: `sim.Component` is no longer an event `Handler`; it is a named, hookable port owner with port notifications (`sim/component.go:10-23`). Many V5 components use `modeling.Component[Spec, State]` with in-place state update semantics (`modeling/component.go:7-28`, `modeling/component.go:60-71`).
8. **Queueing/pipeline changes**: V5 queueing is generic and JSON-state-oriented (`queueing/buffer.go:15-23`, `queueing/pipeline.go:3-18`). Any mgpusim code depending on untyped V4 pipeline/buffer APIs should be budgeted as a behavior-preserving rewrite.
9. **Tracing/monitoring stack changes**: use `tracing` for task hooks/DB tracing (`tracing/api.go:26-45`, `tracing/dbtracer.go:56-70`), `monitoring` for live server integration (`monitoring/doc.go:1-14`, `monitoring/monitor.go:140-176`), and `daisen` for trace replay/storage views (`daisen/trace.go:76-129`). Do not plan this as a one-line import rename.
10. **Cache/system wiring deltas**: V5 memory builders are mixed: some return thin wrappers such as `*idealmemcontroller.Comp` (`mem/idealmemcontroller/builder.go:105-146`, `mem/idealmemcontroller/comp.go:8-15`), while cache builders return `*modeling.Component[Spec, State]` (`mem/cache/writeback/builder.go:216-220`, `mem/cache/writethroughcache/builder.go:202-220`). System builders must handle both shapes.
11. **Checkpoint/save-load precondition**: if mgpusim depends on checkpointing, it must satisfy V5 quiescence and built-topology requirements; V5 `Simulation.Save` refuses non-empty port buffers and does not serialize event queues/connections (`simulation/saveload.go:54-60`, `simulation/saveload.go:292-307`), while `Load` requires a pre-built topology and `SerialEngine` (`simulation/saveload.go:169-210`).

---

## 3) Phased Migration Sequence (Effort + Risk)

| Phase | Goal | Rough effort | Risk | Akita V5 audit adjustment |
|---|---|---:|---|---|
| **P0. Preconditions** | Confirm V5 readiness gates and migration branch strategy | 0-1 week (parallel prep) | Blocker if unmet | Must include tag/freeze decision, current green Akita baseline evidence (`go test ./...`), and downstream mgpusim validation criteria. |
| **P1. Mechanical foundation** | `go.mod` + import path conversion + package discovery + compile triage | 2-4 weeks | Medium | Import paths resolve, but builder/component/package shape changes make this more than a low-risk rename (`go list ...`; `sim/component.go:10-23`; `mem/cache/writeback/builder.go:216-220`). |
| **P2. Type and message core** | Time/ID conversions + response/message metadata migration | 4-6 weeks | Medium | Confirm all string/float task/message assumptions are converted to `uint64`/picosecond semantics (`sim/msg.go:8-17`, `sim/freq.go:5-24`). |
| **P3. Protocol and dataflow** | Control protocol rewrite + queueing/pipeline conversion + event model fixes | 3-5 weeks | Medium-high | Queueing and control are API and behavior migrations, not only symbol renames (`mem/protocol.go:84-112`, `queueing/pipeline.go:70-94`). |
| **P4. Integration wiring** | Component interface alignment, cache/system builder updates, monitoring/tracing integration | 3-5 weeks | High | Raised from medium because V5 uses mixed wrapper/generic component builder returns and monitoring/tracing split APIs (`mem/idealmemcontroller/builder.go:105-146`, `monitoring/monitor.go:53-104`). |
| **P5. Validation and parity** | Test repair, benchmark sanity checks, behavior/perf comparison vs V4 | 2-3 weeks | Medium-high | Preserve the current green Akita `go test ./...` baseline, then prove downstream mgpusim compile, smoke, and parity behavior against V4 references. |

**Expected total:** still ~14-24 person-weeks, but treat the low end as reachable only after P0 release/tag readiness is settled and downstream mgpusim validation gates are defined.

---

## 4) Repository Strategy Recommendation: Subfolder vs Separate Repo

### Option A: Temporary subfolder in this repo

- **Pros:** easier atomic edits when Akita APIs and mgpusim code must change together.
- **Cons:** module-path/history confusion, repository bloat, ownership boundary blur, CI scope creep. The Akita module is already `github.com/sarchlab/akita/v5` (`go.mod:1`), so embedding downstream modules would complicate Go module boundaries.

### Option B: Keep migration in separate `mgpusim(-dev)` repos with `replace` directives

- **Pros:** preserves module boundaries/history, keeps CI/release ownership clear, aligns with standard Go cross-repo development.
- **Cons:** requires coordination across repositories and temporary replace-based local wiring.

### Recommendation (explicit)

Use **Option B (separate repos with temporary `replace` to local Akita V5)** as the default migration workflow.

**Rationale:**

1. `mgpusim-dev` is already the natural integration workspace.
2. Most work is mgpusim-side adaptation, not long-lived Akita API redesign.
3. It avoids monorepo churn while preserving clean release/version semantics.
4. `replace` can be removed once an appropriate Akita V5 tag/release is consumed; no current `v5*` tag was visible via `git ls-remote --tags origin 'v5*'`, so a temporary replace/pseudo-version plan is a practical prerequisite.

---

## 5) Preconditions / Gates Before Implementation Starts

Migration implementation should not begin until these are true or explicitly accepted as ongoing gates:

1. **Akita V5 release readiness gate (future):** migration target API is stable enough for downstream ports (beta/tag or equivalent freeze point). Evidence need: visible tag/freeze commit; current status still has no `v5*` tags via `git ls-remote --tags origin 'v5*'`.
2. **Akita baseline-health gate (currently green):** preserve the current `go test ./...` Akita baseline before and during downstream migration work. Generated mocks are part of the repo merge script (`run_before_merge.sh:9-18`) and package `//go:generate mockgen` directives exist (`mem/trace/generate.go:3`, `mem/vm/gmmu/generate_mocks.go:1-6`, `mem/vm/tlb/tlb_suite_test.go:12`), so downstream migration branches should keep generated test fixtures present or run generation before validation.
3. **Tracing/monitoring compatibility gate:** required mgpusim tracer/reporting/live-monitor capabilities must be mapped explicitly across `tracing`, `monitoring`, and `daisen`, not assumed to be a direct `monitoring` -> `daisen` rename (`tracing/api.go:26-45`, `monitoring/monitor.go:90-104`, `daisen/trace.go:122-129`).
4. **Control/cache API gate:** CP/runner cache-control flows must be rewritten around `mem.ControlReq`/`ControlRsp` (`mem/protocol.go:84-112`) and validated against both wrapper-returning and generic-returning memory builders (`mem/idealmemcontroller/builder.go:105-146`, `mem/cache/writethroughcache/builder.go:202-220`).
5. **Tooling/CI gate:** migration branch strategy, test matrix, and baseline validation criteria are defined in advance. Required commands should include at least `go list` package discovery, current Akita `go test ./...`, generated-mock preservation/regeneration checks, and downstream mgpusim smoke/acceptance tests.
6. **Downstream mgpusim validation gate (future):** mgpusim compile, smoke, representative benchmark, and timing-memory acceptance criteria are selected before import/API conversion begins, then executed after each migration phase.
7. **Checkpoint gate if applicable:** if mgpusim uses checkpoint/save-load, define quiescence, topology rebuild, storage ownership, and SerialEngine constraints before porting (`simulation/saveload.go:54-60`, `simulation/saveload.go:169-210`, `simulation/saveload.go:292-307`).
8. **Scope gate:** agreement that migration starts in `mgpusim-dev` first, then lands in public `mgpusim` after stabilization.

---

## 6) Validation Plan for Follow-on Migration Work

The follow-on implementation milestone should record command evidence separately for Akita and mgpusim:

1. **Akita package/API discovery:** `go list ./sim ./mem ./noc/directconnection ./queueing ./monitoring ./daisen ./simulation ./tracing ./modeling` (passes in this audit).
2. **Akita baseline:** `go test ./...` is expected to pass in the current checkout and should stay green before downstream handoff.
3. **Akita merge-equivalent check:** `./run_before_merge.sh` or a scoped equivalent if full merge checks are too expensive; this script runs `go generate ./...`, `go build ./...`, `golangci-lint run ./...`, and `ginkgo -r` (`run_before_merge.sh:5-18`).
4. **mgpusim compile gate:** all downstream packages compile after import/API conversion.
5. **mgpusim behavior gate:** representative GPU kernels/benchmarks and timing-memory acceptance tests pass against V4 reference outputs within agreed tolerances.
6. **monitoring/tracing gate:** a sample run produces usable task traces through `tracing.DBTracer` and live/replay endpoints through `monitoring`/`daisen` (`simulation/builder.go:102-127`, `monitoring/monitor.go:140-176`).
7. **checkpoint gate if used:** Save/Load tests prove quiescent-only checkpoint behavior and restored ID/time semantics (`simulation/saveload.go:54-60`, `simulation/saveload.go:192-210`).

---

## 7) Explicit Milestone Boundary

This milestone delivers **planning documentation only**.

- No source migration
- No behavior change
- No production code edits

Implementation work is deferred to follow-on milestones/phases after the above gates are met.
