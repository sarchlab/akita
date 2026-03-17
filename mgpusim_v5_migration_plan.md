# MGPUSim V5 Migration Plan (Milestone #660)

## Purpose

This document defines a **planning-only** migration strategy for porting both `sarchlab/mgpusim` and `sarchlab/mgpusim-dev` from Akita V4 to Akita V5.

> **Out of scope for this milestone:** no production code migration is performed here. This is sequencing and risk planning only.

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

| Subsystem area | Typical Akita V4 usage | Migration impact |
|---|---|---|
| `amd/timing/cu`, `amd/timing/cp` | `sim`, `mem/*`, `pipelining`, tracing, control messages | Very high |
| `amd/driver`, `amd/protocol` | message IDs, response matching, `MsgMeta`, VM/mem protocols | High |
| `amd/samples/runner` and timing configs | system wiring, monitoring/reporting/tracing, cache builders | High |
| `amd/timing/mem/*`, `rdma`, `rob` | pipeline/buffer flow + message protocol updates | Medium-high |

Dependency mapping highlights to carry into implementation:

- `akita/v4/sim` -> `akita/v5/sim`
- `akita/v4/mem/mem` -> `akita/v5/mem`
- `akita/v4/sim/directconnection` -> `akita/v5/noc/directconnection`
- `akita/v4/pipelining` -> `akita/v5/queueing`
- `akita/v4/monitoring` -> `akita/v5/daisen`

---

## 2) V4 -> V5 Breaking-Change Categories

The port should be tracked by category, not by file, to reduce regressions:

1. **Import/module path updates** (mechanical rename from v4 paths to v5 paths).
2. **Time and frequency type migration** (`VTimeInSec` and `Freq` now integer-based; remove float-time assumptions).
3. **ID model migration** (message/task IDs and response matching shift from string-style handling to uint64 semantics).
4. **Message model changes** (response linkage through `MsgMeta.RspTo`, removed/changed legacy message interfaces and builder assumptions).
5. **Control protocol unification** (flush/restart/control flows converge to V5 control request/response patterns).
6. **Event serialization/handler identity changes** (event handler references move to handler-ID driven model).
7. **Component model changes** (V5 modeling patterns and updated component/handler expectations around ticking/event handling).
8. **Queueing/pipeline changes** (`pipelining` abstractions replaced by V5 generic queueing/pipeline types).
9. **Tracing/monitoring stack changes** (`monitoring` -> `daisen`, tracer availability/compatibility gaps).
10. **Cache/system wiring deltas** (builder return types and cache package differences that affect runner/timing configs).

---

## 3) Phased Migration Sequence (Effort + Risk)

| Phase | Goal | Rough effort | Risk |
|---|---|---:|---|
| **P0. Preconditions** | Confirm V5 readiness gates and migration branch strategy | 0-1 week (parallel prep) | Blocker if unmet |
| **P1. Mechanical foundation** | `go.mod` + import path conversion + obvious API signature cleanup | 2-4 weeks | Low |
| **P2. Type and message core** | Time/ID conversions + response/message metadata migration | 4-6 weeks | Medium |
| **P3. Protocol and dataflow** | Control protocol rewrite + queueing/pipeline conversion + event model fixes | 3-5 weeks | Medium-high |
| **P4. Integration wiring** | Component interface alignment, cache/system builder updates, daisen integration | 3-5 weeks | Medium |
| **P5. Validation and parity** | Test repair, benchmark sanity checks, behavior/perf comparison vs V4 | 2-3 weeks | Medium |

**Expected total:** ~14-24 person-weeks (depending on tracer/control/cache compatibility and test churn).

---

## 4) Repository Strategy Recommendation: Subfolder vs Separate Repo

### Option A: Temporary subfolder in this repo

- **Pros:** easier atomic edits when Akita APIs and mgpusim code must change together.
- **Cons:** module-path/history confusion, repository bloat, ownership boundary blur, CI scope creep.

### Option B: Keep migration in separate `mgpusim(-dev)` repos with `replace` directives

- **Pros:** preserves module boundaries/history, keeps CI/release ownership clear, aligns with standard Go cross-repo development.
- **Cons:** requires coordination across repositories and temporary replace-based local wiring.

### Recommendation (explicit)

Use **Option B (separate repos with temporary `replace` to local Akita V5)** as the default migration workflow.

**Rationale:**

1. `mgpusim-dev` is already the natural integration workspace.
2. Most work is mgpusim-side adaptation, not long-lived Akita API redesign.
3. It avoids monorepo churn while preserving clean release/version semantics.
4. `replace` can be removed once an appropriate Akita V5 tag/release is consumed.

---

## 5) Preconditions / Gates Before Implementation Starts

Migration implementation should not begin until these are true:

1. **Akita V5 release readiness gate:** migration target API is stable enough for downstream ports (beta/tag or equivalent freeze point).
2. **Tracing compatibility gate:** required tracer/reporting capabilities used by mgpusim are available (or an approved replacement plan exists).
3. **Control/cache API gate:** control protocol and cache builder semantics needed by CP/runner are documented and settled.
4. **Tooling/CI gate:** migration branch strategy, test matrix, and baseline validation criteria are defined in advance.
5. **Scope gate:** agreement that migration starts in `mgpusim-dev` first, then lands in public `mgpusim` after stabilization.

---

## 6) Explicit Milestone Boundary

This milestone delivers **planning documentation only**.

- No source migration
- No behavior change
- No production code edits

Implementation work is deferred to follow-on milestones/phases after the above gates are met.
