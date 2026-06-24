# In-page help screenshots

Illustrating screenshots embedded in the in-page help modals (`src/components/HelpTopics.tsx`).
They are **captured headlessly from a running Daisen**, not hand-drawn, so they
stay faithful to the actual UI.

| File | Source view | Element clipped |
|------|-------------|-----------------|
| `metrics.png` | `/` (index) | the first component chart card (`[data-widget-name]`'s parent) — chart + per-chart legend |
| `components.png` | `/` (index) | the Components widget grid (`[data-widget-name]`'s nearest `.grid`) |
| `tasks.png` | `/component?name=<component>` | the task-count chart (`.daisen1-count-view`) |
| `blocking.png` | `/component?name=<component>` | the blocking-reasons chart (`.daisen1-metric-view`) — use a component that has blocking reasons, e.g. `L1Cache.Top.incoming` |
| `task-tree.png` | `/component?name=<c>&taskid=<id>&starttime=<t>&endtime=<t>` | the selected-task panel (`.daisen1-task-view`) — pick a `req_in` task that has a parent and sub-tasks, and a window spanning them |

## Re-capturing

When the UI changes, regenerate these from a live Daisen:

1. Run Daisen on a trace (`daisen2 -sqlite <trace> -http localhost:<port>`) and a
   headless Chrome with remote debugging enabled.
2. For each row above: navigate to the view, wait for `data-daisen-ready`, read the
   element's `getBoundingClientRect()`, and `Page.captureScreenshot` with that
   `clip` (use `deviceScaleFactor: 2` for a crisp image).
3. Replace the PNG here. Vite hashes it into the bundle on the next build.
