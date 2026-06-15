# DaisenBot Agentic Upgrade — Design & Implementation Plan

**Status:** Draft for review · Phase 1 Workstreams A–E implemented (ComponentPage write-path + in-browser SVG check land in Phase 5) · **Last updated:** 2026-06-15

This document captures the design decisions and phased plan for turning DaisenBot
from a single-shot Q&A proxy into a tool-using agent that can *investigate* an
Akita simulation trace.

---

## 1. Goal

Let DaisenBot **investigate** a trace instead of merely summarizing a CSV it was
handed: query the trace data, read the simulator source to interpret it, run code
to summarize data, and look at Daisen's own visualizations — then answer in text,
with a visible, navigable record of what it did.

**Where we are today** (post-PR #381, `daisen2/internal/httpapi/chat.go`):
DaisenBot is a single-shot, OpenAI-compatible `/chat/completions` proxy. For one
turn it prepends a trace-CSV header (`buildAkitaTraceHeader`) plus a
`beforehandprompt.txt` system prompt to the user's message, makes one model call,
and relays the response. Provider/model/key are supplied by the frontend; the
backend holds no credentials.

**The ceiling we are removing:** the current bottleneck is *data access*, not
reasoning. The model only sees the CSV of the user's pre-selected window and
cannot fetch more — if the answer lives in a parent task, an upstream component,
or a wider time range, it is blind. No amount of "think step by step" recovers
data that was never put in context. This plan makes the model pull what it needs.

---

## 2. Core principles

1. **Tools are inputs; text is the only output.** The agent pulls *data*, *code*,
   and *visualizations* in as observations and emits only a text answer to the user.
2. **Raw data never crosses into the LLM.** Tools return summaries, schemas,
   samples, and images — never row dumps. Heavy data stays server-side (SQL row
   caps; Python summarization).
3. **Daisen is the visualization engine.** Every view is URL-encoded. The agent
   "draws" by generating a URL; the **frontend** renders it — as a clickable link
   for the human, and as an off-screen capture for the agent. No bespoke charting,
   no server-side rendering.
4. **Simplicity over generality.** Agents are always triggered from the chat
   panel, so a live browser is always present. We use it as the renderer and drop
   headless/server-side rendering entirely.

---

## 3. Architecture

### 3.1 The agent loop

Backend-orchestrated, streamed over SSE. `httpChatProxy` evolves from a one-shot
relay into a loop:

```
call provider with tool schemas
  → receive tool_calls
  → execute tools, append observations
  → repeat until the model returns a final answer
  → stream the final text to the user
```

**Lifecycle:** the loop is bound to the chat session's SSE connection. If the
user closes the tab (the connection drops), the backend **cancels** the loop; the
user re-triggers it. This is what lets the frontend be the renderer (§3.2) without
a fallback.

### 3.2 Tools (all inputs)

| Tool | Executes in | Returns | Guardrails |
|---|---|---|---|
| `data_query(sql, limit)` | backend (read-only SQLite) | capped rows / counts / stats | SELECT-only · forced `LIMIT` · row+byte cap · statement timeout · block `ATTACH`/`PRAGMA`/writes |
| `code_search` / `code_read` | backend | Akita source text | scoped to the akita repo · version-matched to the trace's commit if recorded · reuses the existing SSRF-guarded fetch |
| `run_python(code)` | backend sandbox | stdout summary (+ emitted artifacts) | isolated runtime · read-only trace handle · library allowlist · no network · CPU/mem/wall limits |
| `daisen_view(url)` | **frontend** (off-screen) | rendered PNG observation + user-facing link | URL validated/normalized against the view schema · render-ready gate before capture |

`daisen_view` is the **only** tool whose execution delegates to the frontend: the
backend pushes a render request down the SSE channel, the frontend renders the
view off-screen, captures it (SVG-serialize preferred; `html2canvas` fallback),
and POSTs the PNG back, correlated by a request id. All other tools execute in the
backend.

### 3.3 Data flow

```
LLM (needs tool-calling; + vision for daisen_view)
  │   raw trace data NEVER crosses this line — only summaries & images do
  ├─ data_query(sql, limit)   → capped rows / counts / stats        (text)
  ├─ code_search / read       → Akita source                        (text)
  ├─ run_python(code)         → stdout summary                      (text)
  └─ daisen_view(url)         → off-screen frontend render          (image)
                                 (+ clickable link to the human)
  → final text answer
```

The Python sandbox is the hub: it can read the trace (read-only handle), compute
aggregates reliably *in code* (not token-by-token), and keep large data off the
wire to the LLM.

### 3.4 Model capability gating

Agent mode requires a model that supports **tool-calling**; `daisen_view`
additionally requires **vision**. Daisen allows any OpenAI-compatible endpoint and
free-text model id, so these capabilities are not guaranteed. When absent,
**degrade gracefully**: fall back to today's single-shot chat, and/or drop the
viz tool. The model picker advertises which mode is available.

### 3.5 Visible thinking ("the tour")

Each step streams into the chat as a record: the tool used, a one-line rationale,
and — for `daisen_view` — a **thumbnail + clickable link** that jumps the user's
own browser to that exact view. This is the visible-reasoning feature, a trust
mechanism, and a navigable trail at once. Default-on, collapsible.

---

## 4. Agent loop & investigation strategy

§3.1 gave the *mechanical* loop (call → tool_calls → repeat). This section gives the
**control structure** and **reasoning strategy** on top of it — the part that decides
whether the agent investigates competently or just wanders.

### 4.1 Control structure

One **hypothesis-driven investigation loop**, a single agent. We impose a light
investigative shape rather than a free ReAct loop (which over-queries, stops early,
or loops forever) — but **not** a manager-of-subagents (latency, cost, complexity,
against the simplicity bar).

```
Front door (implicit triage — no separate model):
  trivial / definitional   → answer directly (0 tools, or 1 lookup)
  ambiguous                → ask ONE focused clarifying question
  investigative            → enter the loop ↓

Investigation loop:
  1. Orient     — cheap "vital-signs" sweep over the in-scope components/window
  2. Hypothesize— emit a RANKED candidate-cause list (seeded by the catalog, §4.4)
  3. Test       — gather the minimal evidence that distinguishes the top hypothesis
  4. Iterate    — next hypothesis / refine; stop on support or budget (§4.6)
  5. Report     — text answer + the tour (links/thumbnails) of what was checked
```

### 4.2 Front door / triage

**Implicit, not a separate router LLM.** Native tool-calling already lets the model
answer directly when it needs no tool, so a simple question short-circuits the loop
with zero tool round-trips. The system prompt authorizes three outcomes so trivial
*and* ambiguous cases never spiral into tool calls:

- **Direct answer** — definitional/summary questions ("what does the `read-miss`
  Kind mean?", "summarize this view"). Answerable from the primed trace-context
  header (kept from today, §1) or one lookup.
- **Clarifying question** — underspecified asks ("why is it slow?" with no component
  or baseline). Ask ONE focused question; default the scope to the current view's
  selected components + time window when reasonable rather than always asking.
- **Investigate** — enter the loop.

An *explicit* router LLM is deferred (§4.7); it earns its keep only at volume (cost
control), for explicit mode selection, or for abuse rejection.

### 4.3 The investigation loop

- **Orient.** Before hypothesizing, run a cheap vital-signs sweep (§4.5) to get the
  lay of the land — grounds hypotheses in the actual trace, not priors.
- **Hypothesize.** Emit a ranked candidate-cause list (§4.4) as an explicit
  *structured* artifact, not free-form prose. Rank by prior-likelihood ×
  cheapness-to-test.
- **Test.** Take the top hypothesis; collect the *minimal distinguishing* evidence
  (§4.5); confirm or refute. One hypothesis at a time keeps the tour legible.
- **Iterate.** Move to the next hypothesis or refine. Stop when one is supported with
  evidence, or budgets are hit (§4.6).
- **Report.** Text answer citing the supporting evidence, with the visible tour
  (§3.5). If nothing is conclusive, say so and report what was *ruled out* — never
  fabricate a cause.

### 4.4 Hypothesis generation & the failure-mode catalog

**The agent generates hypotheses — seeded by a curated catalog of known Akita
bottleneck patterns.** A general model free-forming causes misses arch-specific
failure modes; the catalog is the single highest-leverage quality lever, and it is
just content (a prompt section or a `list_known_failure_modes` retrieval).

The hypothesis list is a **structured, schema-enforced artifact** (`id`,
`description`, `catalog_pattern`, `evidence_to_collect`, `status ∈ {untested,
supported, refuted}`). Benefits: auditable, drives the tour, and yields a natural
stopping criterion (top-K tested).

**Catalog schema:** `pattern · symptom-in-trace · distinguishing-evidence-to-collect`.

**Starter set — seeded from general computer-architecture / memory-system knowledge.
⚠️ DOMAIN REVIEW NEEDED (owner: Yifan) to validate and expand against Akita's actual
components and milestone vocabulary:**

| Pattern | Symptom in trace | Distinguishing evidence |
|---|---|---|
| Cache capacity/conflict thrashing | high miss rate; working set > capacity | miss ratio over time; reuse distance; set-conflict skew |
| MSHR / outstanding-request exhaustion | misses queue though the cache isn't "busy" | in-flight miss count vs. MSHR limit; wait-for-MSHR time |
| Queue backpressure / buffer-full | upstream stalls while downstream is full | buffer occupancy over time; stall edges to the full buffer |
| DRAM bank conflicts | serialized accesses to the same bank | per-bank access timeline; same-bank back-to-back gaps |
| Row-buffer thrashing | frequent activate/precharge; low hit rate | row-buffer hit ratio; activate frequency |
| Bandwidth saturation | a link/port at ~100% utilization | per-port utilization; queueing delay vs. service time |
| Head-of-line blocking | one slow request stalls others behind it in a FIFO | per-entry wait vs. service; FIFO depth at stall |
| TLB miss / page-walk stalls | translation stalls precede the access | TLB-miss timeline; page-walk durations |
| Arbitration/contention | requests wait for grant at a shared arbiter | grant latency; requesters-per-cycle at the arbiter |
| Load imbalance | one component hot while peers idle | per-component utilization spread |
| Latency not hidden (low MLP/ILP) | stalls despite spare resources | outstanding-work count vs. stall time |

### 4.5 Evidence-gathering policy

- **Vital-signs first** (the Orient pass), computed as aggregates in SQL/Python —
  never raw rows:
  - per-component task count, busy/idle time, utilization;
  - per-`Kind` duration distribution (p50/p95/max);
  - in-flight concurrency over time (start/end overlap);
  - dependency wait gaps (child blocked on parent / upstream component);
  - time-in-milestone breakdown.
- **Hypothesis-driven drill-down.** Each subsequent query is chosen to confirm/refute
  a specific hypothesis; collect the minimum that distinguishes candidates, not
  everything. Drill coarse → suspicious region/component, following `ParentID` up/down
  to the thing a stalled component waits on (the data today's single-shot bot can't
  reach).
- **Viz is evidence too.** Some patterns (bursts, periodicity, gaps) are easier to
  *see* via `daisen_view` than to express in SQL.
- **Budgeted.** Row/byte caps per query; summarize in Python so raw data never
  reaches the LLM (§2).

### 4.6 Loop control, budgets & termination

- **Caps:** max iterations, max tool calls, an overall wall-clock ceiling (build on
  the existing 10-min client ceiling), and a token budget.
- **Stopping criteria:** a hypothesis is supported with evidence; OR the top-K
  hypotheses are all refuted; OR a budget is hit.
- **Graceful degradation:** on exhaustion, report the best-supported partial finding
  and what was ruled out — never loop silently or fabricate. Surface that a limit was
  hit (no silent truncation, per the visible-tour principle).

### 4.7 Deferred (and when to add)

- **Explicit router LLM** — when request volume makes loading full context on trivial
  questions costly, or when distinct modes need distinct loops.
- **Manager + worker fan-out** — for genuinely parallel work (compare N components,
  broad audits): a planner dispatches one worker per component/hypothesis and
  synthesizes. Reserved for when a single loop is demonstrably too serial.

### 4.8 Phasing

The strategy lands incrementally: **Phase 2** introduces the front door + the
Orient→Hypothesize→Test loop using the `data` tool and a first catalog; **Phases 3–5**
add evidence tools (code, python, viz); **Phase 6** hardens budgets/termination and
the degradation matrix.

---

## 5. Phase roadmap

Each phase is decomposed into workstreams (A–E style) when we reach it; Phase 1 is
detailed in §6.

| Phase | Title | Summary | Depends on |
|---|---|---|---|
| **1** | View-URL contract + render-ready signal | Make every view losslessly reconstructable from its URL, and expose a programmatic "view is fully rendered" signal. Renderer-agnostic; independently useful as better link-sharing. | — |
| **2** | Agent-loop skeleton + tool-calling + `data` tool | Turn `httpChatProxy` into a streamed multi-step tool-calling loop; land the guarded read-only `data_query`; add capability-gating with single-shot fallback. Includes the front door + Orient→Hypothesize→Test loop (§4) and a first failure-mode catalog. A working "agent that can query the trace" — the biggest single value jump. | — |
| **3** | `code` tool | `code_search` + `code_read` over Akita source so the agent can interpret Kinds/milestones. | 2 |
| **4** | Python sandbox tool | Sandboxed `run_python` for summarizing data without shipping raw rows. Isolated as its own phase for the runtime/isolation decision. | 2 |
| **5** | Viz-perception tool + visual tour | `daisen_view`: frontend off-screen render → image observation; clickable links + thumbnails. Extends gating to require a vision model. | 1, 2 |
| **6** | Hardening | Loop bounds (max iters / wall-clock), cross-tool context budgeting/pagination, graceful-degradation matrix (model caps × installed runtimes), answer-quality eval harness. | 2–5 |

**Parallelism:** Phases 1 (frontend) and 2 (backend) touch different subsystems
and can proceed in parallel. We start with Phase 1 because the view-URL contract
is the prerequisite for *both* human links and the agent's perception (Phase 5),
and it ships value on its own.

---

## 6. Phase 1 — detailed

**Goal:** every meaningful view is fully reconstructable from its URL alone, and a
programmatic "this view has finished rendering" signal exists.

**Non-goals (Phase 1):** no LLM/agent loop, no capture/round-trip wiring, no
Python sandbox. Only: lossless URL encoding + the render-ready signal. (Both are
independently useful for plain link-sharing.)

### Workstream A — View-state inventory & gap audit *(the artifact to review)*

Enumerate every route/page and, for each, list **all state that affects what is
drawn**, classifying each field as `in-URL` / `react-state-only (gap)` /
`derived (no-op)`.

**Status: completed 2026-06-15** (audited `daisen2/static/src`). Routing is
react-router v6 (`<Routes>` / `useSearchParams`); all parameterization is via query
string — no path params. Full route set:

| Route | Page | URL state today |
|---|---|---|
| `/` **and** `/dashboard` | DashboardPage | **none** — reads/writes no params |
| `/component` | ComponentPage | `name`, `taskid`, `starttime`, `endtime` |
| `/task` | TaskChartPage | `id`, `where` |

**`/component` (ComponentPage)** — params read at `:699-703`, but writes use raw
`window.history.replaceState` (`:762-766`, `:882-885`), bypassing react-router; the
range write is debounced 1 s.

| Field | Source | Action |
|---|---|---|
| component `name` | URL | keep — route write through `viewState`/`setSearchParams` |
| selected `taskid` | URL-seeded state | keep — same fix |
| view range `starttime`/`endtime` | URL-seeded state | keep — **fix the `replaceState` bypass**; keep 1 s debounce |
| hoveredTask, highlightedKey, selectedTaskSeed, measured `size`, drag refs | react-state / refs | **ephemeral — do not encode** |
| metric type (`"ConcurrentTask"`, `:722`) | hardcoded | n/a — no selector to encode |

**`/task` (TaskChartPage)** — `id`,`where` read at `:22-23`; uses real
`setSearchParams`. No time-range/zoom control (always full sim range).

| Field | Source | Action |
|---|---|---|
| task `id` | URL | keep |
| component filter `where` | URL | keep — **fix:** selecting `where` clobbers all params incl. `id` (`:87`) |
| kind filter `kind` | react-state (`:26`) | **lift** → new `kind` param |
| selected task (detail pane) | react-state (`:27`) | **lift** → optional `sel` param (browse-mode selection) |
| GanttChart `selectedId` | chart-local (`GanttChart.tsx:54`) | **fix:** unify with page `selectedTask`; encode via `sel` |
| taskInput draft (`:24`) | react-state | ephemeral — do not encode |

**`/dashboard` (DashboardPage)** — **reads and writes no URL params at all**; the
entire view is react-state. Biggest gap.

| Field | Source | Action |
|---|---|---|
| view range `starttime`/`endtime` (`:71`) | react-state | **lift** |
| filter text (`:74`) | react-state | **lift** → `filter` |
| pagination `page` (`:75`) | react-state | **lift** → `page` |
| primary axis metric (`:76`) | react-state | **lift** → `primary` |
| secondary axis metric (`:77`) | react-state | **lift** → `secondary` |
| single-widget focus | NEW | **add** `widget=<component>` → render only that chart full-view (agent's single-chart unit) |
| measured grid `size` (`:78`) | viewport-derived | ephemeral (function of window size) — do not encode |
| `/` vs `/dashboard` (`App.tsx:11-12`) | routing | **canonicalize** to `/dashboard` (redirect `/`) |

**Cross-cutting:** simulation range, segments, and component-name list are server
hooks (`useSimulationRange` / `useSegments` / `useComponentNames`) — reconstruct from
the server, not the URL (no-op). There is **no React context / store / global**
holding view state, and **no multi-selection anywhere** (all selections single-valued).

**Resulting canonical schema (input to Workstream B):**

```
/dashboard : starttime? endtime? filter? page? primary? secondary?
             widget?        # when set: render ONLY that component's chart, full-view
/component : name  taskid?  starttime?  endtime?
/task      : id?  where?  kind?  sel?
```

All single-value params; the existing names (`name`, `taskid`, `starttime`,
`endtime`, `id`, `where`) are preserved for link back-compat — only new params are added.

**Decisions (accepted 2026-06-15):**
1. **Dashboard scope** — encode `filter`, `page`, `primary`, `secondary`. ✅
2. **Canonical route `/dashboard`**, redirect `/` → `/dashboard`. ✅
3. **Fix the `replaceState` bypass** on ComponentPage so URL↔state round-trips
   through react-router (required for deterministic URL→render). ✅
4. **Single-widget URL mode** — `/dashboard?widget=<component>` renders only that
   component's chart, full-view; the grid collapses to one widget. This is the
   agent's natural single-chart perception unit for `daisen_view` (§3.2), and it is
   also user-shareable. A per-widget "focus" affordance sets the param. ✅

This **resolves open questions #1, #3, and #5** (see §7).

### Workstream B — Canonical URL schema (the contract)

- Per-route param vocabulary, names, formats per the schema established in
  Workstream A (time as ns float; component as full-name string; all single-value —
  no multi-select exists today; zoom expressible via start/end).
- **Normalization** so identical views → identical canonical URLs (param order,
  defaults, rounding) — needed later for render caching/dedup.
- **Validation** rules — the Go backend reuses these to validate LLM-generated
  URLs before rendering or before handing a link to a user.
- **Single source of truth:** a small TS module `viewState.ts` exposing
  `encodeView(state) → string` and `parseView(searchParams) → state`, replacing
  today's scattered `searchParams.get(...)` calls.

### Workstream C — Close the gaps

- Lift each gap from A into the URL via `viewState.ts` + `setSearchParams`, with
  two-way sync (UI→URL, and URL→UI on load/back/forward).
- Use `replace` (not `push`) and **debounce** for continuous interactions
  (drag-zoom, pan) so history isn't spammed. Encode only state that changes "what
  view is shown," not transient hover/tooltip state.
- **Canonicalize** the dashboard route to `/dashboard` and redirect `/` → `/dashboard`.
- **Fix the ComponentPage `replaceState` bypass** — route URL writes through
  react-router (`setSearchParams`) so URL↔state stay in sync and a loaded URL renders.
- **Single-widget mode** — when `widget=<component>` is present, DashboardPage renders
  only that component's `DashboardWidget` full-view (grid/pagination/filter suppressed);
  add a per-widget "focus" control that sets the param. Reuses the existing widget; the
  same `starttime`/`endtime`/`primary`/`secondary` params apply.

**Status (2026-06-15): implemented except the ComponentPage `replaceState` conversion.**
Landed: `viewState.mjs` `encodeSearchParams` + `mergeParams`; `/`→`/dashboard` redirect;
DashboardPage URL-encodes filter/page/primary/secondary/range + single-widget mode +
per-widget focus; DashboardWidget `onFocus`; GanttChart controlled selection;
TaskChartPage lifts `kind`/`sel` and no longer clobbers params on component change.
Verified with `tsc --noEmit`, 35 `node --test` cases, and `vite build` (all green).
**Deferred:** ComponentPage already reconstructs its view from the URL on load (mount
init reads `name`/`taskid`/`starttime`/`endtime`); converting its live `replaceState`
writes to react-router touches the sim-range-follow / resync state machine, so it is best
validated with the app running (alongside Workstream D / a manual smoke test).

### Workstream D — Render-ready signal

- One source of truth for "data loaded **and** SVG committed." Each loading hook
  registers in-flight; when all settle and the chart has painted (post-commit
  frame), set `window.__daisenViewReady = true` **and** `data-daisen-ready="true"`
  on a root node.
- **Reset to false** when a navigation/param change starts a new fetch.
- **Terminal states:** flip ready on empty and error too
  (`ready-ok` / `ready-empty` / `ready-error`) so the off-screen capture never hangs.
- Await `document.fonts.ready` before signaling — SVG text metrics depend on fonts;
  keeps captures deterministic.

**Status (2026-06-15): implemented.** `src/utils/renderReady.mjs` (pure, node-tested)
counts in-flight work via `beginRenderWork()`; `hooks/useRenderReady.ts` wires it into
all five data hooks (`useTraceData`, `useCompInfo`, `useComponentNames`, `useSegments`,
`useSimulationRange`), so every page- and widget-level fetch is tracked. When the count
returns to zero it waits one frame + `document.fonts.ready`, then sets
`window.__daisenViewReady = true` and `<html data-daisen-ready="ok|error">`;
`useRenderReadyOnNavigation()` (in Layout) clears it on route change. Simplification: a
rendered empty view settles as `ready-ok` (not a distinct `ready-empty`) — only
"settled vs in-flight" matters for capture, and `error` is still surfaced. Verified by
`tests/render-ready.test.mjs` (7 cases) + `tsc` + `vite build`; exact capture frame-timing
is finalized with the running app in Phase 5.

### Workstream E — Off-screen render + verification harness

This is where Phase 1 produces a reusable building block for Phase 5's
`daisen_view`:

- Build a hidden render path: given a Daisen URL, mount the view in an off-screen
  container with real layout, wait for the render-ready signal, and confirm a
  non-empty SVG is present. (Capture + backend round-trip come in Phase 5; here we
  prove the view renders headlessly-in-the-tab and signals correctly.)
- Pure unit test: `parseView(encodeView(s)) === s` for representative states.
- In-browser test: load N representative URLs, await `data-daisen-ready`, assert
  non-empty SVG.

**Status (2026-06-15): automated portions landed; in-browser SVG assertion folded into
Phase 5.**
- ✅ Pure unit test — `parseView(encodeView(s)) === s` (`tests/view-state.test.mjs`).
- ✅ Signal lifecycle — `tests/render-ready.test.mjs` proves ready flips correctly
  (false on work/navigation, true on settle, `error` status, idempotent end).
- ⏭ Off-screen mount of a real view + "non-empty SVG" assertion needs a browser and is
  *the same mechanism as Phase 5's frontend capture* (mount a URL off-screen → await
  `data-daisen-ready` → read the SVG), so it is built and tested there rather than
  duplicated. Until then, use the manual smoke checklist.

**Manual smoke checklist** (run `daisen2` against a trace):
1. Visiting `/` redirects to `/dashboard`.
2. Dashboard: zoom/pan and change filter/axes/page → URL updates; reload reproduces the
   exact view; back/forward works.
3. A widget's focus control → `/dashboard?widget=<name>` shows only that chart; "Show all"
   returns to the grid.
4. Task page: set a Kind filter and select a task → `kind`/`sel` appear in the URL and
   reload restores them; the Gantt highlight matches the detail pane.
5. Console: `window.__daisenViewReady` is `false` while a view loads, `true` once settled;
   `<html>` carries `data-daisen-ready`.

### Sequencing & acceptance

- Order: **A → B → (C ∥ D) → E.**
- **Done when:** every view-state field is in-URL or explicitly documented as
  intentionally-ephemeral; round-trip test green; any canonical URL reproduces the
  exact view incl. back/forward; the ready signal reliably flips (incl. empty/
  error) and resets on navigation; the off-screen harness reaches ready with a
  non-empty SVG on all sample URLs.

---

## 7. Open questions / to audit

1. ~~**Multi-select / large selections**~~ — **Resolved (Workstream A):** no
   multi-selection exists in any page; single-value params suffice. Revisit only if
   multi-select is ever added.
2. **Centralization refactor** — OK to introduce `viewState.ts` and route all
   pages through it, or prefer minimal per-page edits to limit blast radius?
3. ~~**Scope line**~~ — **Resolved (Workstream A):** ephemeral =
   hover / legend-highlight / measured-size / draft-text / drag-refs; dashboard grid
   layout is viewport-derived (not encoded). View-defining set is the §6 per-page
   tables.
4. **URL back-compat** — if param names change, existing shared links break. Keep
   aliases?
5. ~~**Inventory completeness**~~ — **Resolved (Workstream A):** confirmed
   `/component`, `/task`, `/dashboard` are the only content routes; plus a
   `/`↔`/dashboard` canonicalization item.
6. **Python sandbox runtime** (Phase 4 decision) — bubblewrap / container / WASM
   as the safe default, with a subprocess fast-path for single-user local mode.
7. **Capture fidelity** (Phase 5) — SVG-serialize is more faithful than
   `html2canvas` for the charts; confirm it covers all view types (or where
   `html2canvas` is the needed fallback).
8. **Triage** (§4.2/§4.7) — stay implicit, or add an explicit router LLM? If
   explicit, at what volume/cost threshold, and what modes would it route to?
9. **Failure-mode catalog** (§4.4) — domain input needed: validate and expand the
   starter set against Akita's real components and milestone vocabulary. Who owns it,
   and does it live in the prompt or behind a `list_known_failure_modes` tool?
10. **Hypothesis artifact** (§4.4) — confirm a structured, schema-enforced hypothesis
    list (vs. free-form prose) is the right contract, and the fields to enforce.

---

## 8. Non-goals

- No headless/server-side rendering (Chromium) — the live frontend is the renderer.
- No bespoke charting (matplotlib/plotly) for agent perception — Daisen renders.
- No persistent server-side credentials — keys remain frontend-supplied per request.
- No support for agents running with no browser session attached
  (background/scheduled agents) — out of scope by the simplicity decision.

---

## Appendix — Decisions log

| Decision | Rationale |
|---|---|
| Build a tool-using agent, not just prompted CoT / a reasoning model | The ceiling is data access, not reasoning; the agent must fetch its own data. |
| data / code / viz = inputs; text = output | Clarifies the tool model; viz is perception, not UI actuation. |
| Backend-orchestrated loop over SSE | Owns the SQLite reader and SSRF-guarded fetch; one place for limits; key already arrives per-request. |
| `daisen_view` renders in the frontend, not headless Chromium | Agents always launch from the chat panel ⇒ a live browser is guaranteed. No Chromium dependency; the agent sees what the user sees. Tab close ⇒ cancel the loop. |
| Daisen views are URL-encoded; the agent generates URLs | Reuses 100% of Daisen's viz; the URL is a shareable, replayable artifact for both human and agent. |
| Show the thinking process (links + thumbnails) | Visible reasoning + trust + a navigable trail, near-free over SSE. |
| Guarded read-only `data_query` with hard caps | Powerful over a small fixed schema; SELECT-only + LIMIT + timeout bounds risk; computing in SQL avoids token-arithmetic hallucination. |
| Python sandbox for summarization | Keeps raw data off the LLM wire and makes numeric analysis reliable. |
| Capability-gate on tool-calling (+ vision for viz) | Free-text model ids mean capabilities aren't guaranteed; degrade to single-shot. |
| Single hypothesis-driven loop; implicit triage; router/manager deferred (§4) | Matches how trace debugging actually works; avoids multi-agent latency/cost; native tool-calling already short-circuits trivial questions. |
| Hypotheses generated by the agent, seeded by a curated failure-mode catalog, emitted as a structured ranked artifact (§4.4) | A general model misses arch-specific causes; the catalog is the top quality lever; structure makes it auditable and gives a stopping criterion. |
| Evidence is hypothesis-driven, vital-signs-first, budgeted (§4.5) | Minimizes tokens/latency and keeps raw data off the LLM wire. |
| Canonical dashboard route `/dashboard` (redirect `/`) | One canonical URL per view; required for shareable links and deterministic agent rendering. |
| Single-widget URL mode `/dashboard?widget=<component>` | A single focused chart is the agent's natural perception unit for `daisen_view`; also user-shareable. |
