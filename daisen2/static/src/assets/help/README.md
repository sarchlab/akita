# In-page help screenshots

Illustrating screenshots embedded in the in-page help modals (`src/components/HelpTopics.tsx`).
They are **captured headlessly from a running Daisen**, not hand-drawn, so they
stay faithful to the actual UI.

| File | Source view | Element clipped |
|------|-------------|-----------------|
| `metrics.png` | `/` (index) | the first component chart card (`[data-widget-name]`'s parent) — chart + per-chart legend |
| `selector.png` | `/dashboard` | the sidebar (`aside`) — the search box + component-hierarchy tree; expand a branch or two first, then clip the top ~360px |
| `components.png` | `/` (index) | the Components widget grid (`[data-widget-name]`'s nearest `.grid`) |
| `tasks.png` | `/component?name=<component>` | the task-count chart (`.daisen1-count-view`) — "Component tasks" help, zoomed-out |
| `component-tasks.png` | `/component?name=<c>&starttime=<t>&endtime=<t>` | the per-task gantt (`.daisen1-component-view`) — "Component tasks" help, zoomed-in; use a narrow window so the gantt renders (few enough tasks) |
| `blocking.png` | `/component?name=<component>` | the blocking-reasons chart (`.daisen1-metric-view`) — use a component that has blocking reasons, e.g. `L1Cache.Top.incoming` |
| `task-tree.png` | `/component?name=<c>&taskid=<id>&starttime=<t>&endtime=<t>` | the selected-task panel (`.daisen1-task-view`) — "Task hierarchy" help; pick a `req_in` task that has a parent and sub-tasks, and a window spanning them |

## Re-capturing

When the UI changes, regenerate these from a live Daisen:

1. Run Daisen on a trace (`daisen2 -sqlite <trace> -http localhost:<port>`) and a
   headless Chrome with remote debugging enabled.
2. For each row above: navigate to the view, wait for `data-daisen-ready`, read the
   element's `getBoundingClientRect()`, and `Page.captureScreenshot` with that
   `clip` (use `deviceScaleFactor: 2` for a crisp image).
3. Replace the PNG here. Vite hashes it into the bundle on the next build.
