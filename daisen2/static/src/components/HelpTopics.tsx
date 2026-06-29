import InfoButton from "./InfoButton";
import metricsImg from "../assets/help/metrics.png";
import componentsImg from "../assets/help/components.png";
import tasksImg from "../assets/help/tasks.png";
import blockingImg from "../assets/help/blocking.png";
import taskTreeImg from "../assets/help/task-tree.png";
import componentTasksImg from "../assets/help/component-tasks.png";
import selectorImg from "../assets/help/selector.png";
import taskGanttImg from "../assets/help/task-gantt.png";

// HelpTopics are ready-made InfoButtons for the concepts that are hard to grasp at
// a glance, so a view just drops in e.g. <MetricsHelp /> next to the thing it
// explains. Keeping the copy (and the illustrating screenshots) here keeps it
// consistent and out of the view code. The screenshots are captured headlessly
// from a live Daisen; see src/assets/help/README.md to re-capture them.

const Term = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <li>
    <strong>{label}</strong> — {children}
  </li>
);

// Figure shows an illustrating screenshot in a help modal, optionally captioned.
const Figure = ({ src, alt, caption }: { src: string; alt: string; caption?: string }) => (
  <figure className="mb-2">
    <img src={src} alt={alt} className="w-full rounded border border-slate-200" loading="lazy" />
    {caption ? <figcaption className="mt-1 text-xs italic text-muted-foreground">{caption}</figcaption> : null}
  </figure>
);

// MetricsHelp explains the per-component Y-axis metrics (dashboard / widgets).
export function MetricsHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Chart metrics" className={className}>
      <Figure src={metricsImg} alt="A component chart's two metric lines, named in the legend below it" />
      <p>Each line plots one metric for the component over time. Pick them with the Y-axis menus; the color identifies the metric (also shown in the legend).</p>
      <ul className="space-y-1.5">
        <Term label="Incoming Request Rate">requests arriving at the component per unit time.</Term>
        <Term label="Request Complete Rate">requests the component finishes per unit time.</Term>
        <Term label="Average Request Latency">mean time from a request arriving to completing.</Term>
        <Term label="Number Concurrent Task">how many tasks are in flight at once — the component's concurrency.</Term>
        <Term label="Request Buffer Pressure">how many incoming <strong>requests</strong> are sitting in the component's input buffers, waiting to be served.</Term>
        <Term label="Response Buffer Pressure">how many incoming <strong>responses</strong> (to requests the component itself sent) are waiting in its input buffers. A pure client that only issues requests has request pressure ≈ 0 but accumulates responses.</Term>
        <Term label="Pending Request Out">how many requests the component has issued downstream and is still awaiting responses for — its outstanding requests.</Term>
      </ul>
    </InfoButton>
  );
}

// ComponentSelectorHelp explains the dashboard sidebar: the search box and the
// component-hierarchy tree used to choose what the grid shows.
export function ComponentSelectorHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Choosing components" className={className}>
      <Figure src={selectorImg} alt="The dashboard sidebar: a search box above the component-hierarchy tree" />
      <p>The sidebar chooses <strong>which components the dashboard grid shows</strong> — the grid draws one chart per component at the level you pick.</p>
      <p>The tree is the simulation's <strong>component hierarchy</strong>. Each row is a component or a group of them:</p>
      <ul className="space-y-1.5">
        <Term label="Click a name">scopes the grid to that node — the grid switches to its children and the breadcrumb above the grid tracks where you are. Click a breadcrumb, or <strong>All components</strong>, to go back up.</Term>
        <Term label="Triangle">expands or collapses a branch in the tree without changing the grid. A leaf (small dot) is a single <em>one-location-one-kind</em> facet and has nothing to expand.</Term>
      </ul>
      <p>A node with children <strong>aggregates its whole subtree</strong> into one chart — the <strong>Σ N facets</strong> badge on the chart counts the leaf locations summed into it. Scope into the node to break it apart, or open a single chart to drill into just that component.</p>
      <p>The <strong>search box</strong> filters the tree to matching components and jumps the grid straight to their charts — handy when you know a component's name but not where it sits in the hierarchy.</p>
    </InfoButton>
  );
}

// ComponentsOverviewHelp explains the index page's Components widget: residency
// ranking, auto-selected metrics, and the aggregate facet badge.
export function ComponentsOverviewHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Components overview" className={className}>
      <Figure src={componentsImg} alt="The Components overview: four component charts with facet badges and per-chart legends" />
      <p>The components with the highest <strong>residency</strong> — the total time their tasks spend in flight, summed across the run. It is a cheap proxy for where the simulation spends time, so the busiest / most-contended components rank first.</p>
      <p>Each chart's two metrics are <strong>auto-selected</strong> from what the component does:</p>
      <ul className="space-y-1.5">
        <Term label="Request-fulfilling components">caches, memory, translators — anything that serves incoming requests — show <strong>Incoming Request Rate</strong> and <strong>Average Request Latency</strong>.</Term>
        <Term label="Task-executing components">cores and traffic agents — they run work and issue requests but serve none — show <strong>Number Concurrent Task</strong> and <strong>Pending Request Out</strong>.</Term>
      </ul>
      <p>The colored dots below each chart name its two metrics.</p>
      <p>A <strong>Σ N facets</strong> badge means the chart sums the component's whole subtree — its N <em>one-location-one-kind</em> facets (e.g. <code>AT.req_in</code>, <code>AT.Top.incoming</code>, …). Open a chart to drill into the component.</p>
    </InfoButton>
  );
}

// ComponentTaskViewHelp explains the per-task gantt (the "component task view"):
// every task drawn as its own bar, shown when the range holds few enough tasks.
export function ComponentTaskViewHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Component task view" className={className}>
      <Figure
        src={componentTasksImg}
        alt="The per-task gantt: each task drawn as its own bar across time"
        caption="Each task is its own bar; the stacked rows are tasks in flight at the same time."
      />
      <p>This view draws <strong>each of the component's tasks as its own bar</strong> across time. A task is one unit of work — serving an incoming request (<code>req_in</code>), a request it sent out (<code>req_out</code>), the time a message spent in a buffer (<code>incoming_buffer</code> / <code>outgoing_buffer</code>), a pipeline stage, and so on (see <em>Task types</em>).</p>
      <p>Overlapping tasks are stacked into <strong>rows</strong> so they don't hide each other — the number of rows in use at a moment is the component's <strong>concurrency</strong>. <strong>Hover</strong> a bar for its details and <strong>click</strong> one to select it: that opens its <em>task hierarchy</em> above and its full details in the side panel.</p>
      <p>It appears only when the visible range holds <strong>few enough tasks</strong> to draw each one. With more, it is replaced by the <em>Task count</em> chart below — <strong>zoom in</strong> (⌘/Ctrl+scroll, pinch, or drag-select a range) to bring the bars back. Scroll or drag to pan; <strong>Alt+scroll</strong> (or the <em>rows</em> buttons) makes the rows taller or shorter.</p>
    </InfoButton>
  );
}

// TaskCountHelp explains the task-count density chart: the level-of-detail summary
// shown when too many tasks fall in the range to draw each one individually.
export function TaskCountHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Task count" className={className}>
      <Figure
        src={tasksImg}
        alt="The task-count chart: in-flight tasks stacked and colored over time"
        caption="At each time, how many tasks are in flight — stacked and colored by kind."
      />
      <p>The task-count chart is a <strong>summary</strong> of the component's tasks, shown when the range holds too many to draw individually. At each time it stacks <strong>how many tasks are in flight</strong>, so the stack height is the component's <strong>concurrency</strong>, and the colored bands break that down by task kind.</p>
      <p>A stack that stays high with no dips means the component is working at <strong>full capacity</strong>; dips mean it idled. Hover a band to highlight that kind. The legend's <strong>Kind / Kind-What</strong> toggle controls the coloring — by task kind alone, or split further by the message type (e.g. <code>incoming_buffer · ReadReq</code> vs <code>· WriteReq</code>) — recoloring both these bands and the per-task bars.</p>
      <p><strong>Zoom in</strong> (⌘/Ctrl+scroll, pinch, or drag-select a range) until few enough tasks remain that each becomes its own bar in the <em>Component task view</em> above.</p>
    </InfoButton>
  );
}

// TaskTypesHelp explains the task kinds listed in the side-panel Tasks legend —
// the buffer/processing tasks a request flows through (incoming_buffer, req_in,
// req_out, outgoing_buffer, pipeline).
export function TaskTypesHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Task types" className={className}>
      <p>Each colored row is one <strong>kind</strong> of task this component records. A task is one unit of work spanning a stretch of time; the legend's <strong>Kind / Kind-What</strong> toggle splits a kind further by the message it handled (e.g. <code>req_in · ReadReq</code> vs <code>· WriteReq</code>).</p>
      <p>A request flows through a component as a few paired tasks:</p>
      <ul className="space-y-1.5">
        <Term label="incoming_buffer">how long a message waited in a port's <strong>input</strong> buffer — from the moment the connection delivered it until the component admitted (retrieved) it. This time is queueing delay <em>before</em> the component begins working.</Term>
        <Term label="req_in">the component <strong>serving an incoming request</strong>, from admission until completion. This is the component's own processing of that request, and it begins where the <code>incoming_buffer</code> wait ends.</Term>
        <Term label="req_out">a request the component <strong>issued downstream</strong>, from the moment it is sent until the response returns. Serving a <code>req_in</code> typically spawns one or more <code>req_out</code> sub-requests to other components.</Term>
        <Term label="outgoing_buffer">how long a message waited in a port's <strong>output</strong> buffer — from when the component sent it until the connection drained it onto the link. This time is mostly waiting for the link to be free.</Term>
        <Term label="pipeline">a request's traversal of a component-internal <strong>latency pipeline</strong> (e.g. <code>L2Cache.bank</code>), recorded as a sub-task of <code>req_in</code> so the pipeline time is accounted for instead of left as an unexplained gap.</Term>
      </ul>
      <p>Read together they tell a request's story: it sits in <code>incoming_buffer</code>, is served as <code>req_in</code> (often traversing a <code>pipeline</code> and issuing <code>req_out</code> sub-requests), and its replies leave through <code>outgoing_buffer</code>.</p>
    </InfoButton>
  );
}

// TaskGanttHelp explains the task view's main visualization: one task shown with
// its full ancestor chain above and its descendant subtree below, on one time axis.
export function TaskGanttHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Task view" className={className}>
      <Figure
        src={taskGanttImg}
        alt="The task gantt: a current task with its ancestor chain stacked above and subtasks below, on one time axis"
        caption="One task's whole lineage on a single time axis: ancestors above, the current task, subtasks below."
      />
      <p>This view follows <strong>one task through its whole lineage</strong> on a single time axis. The task you opened is the <strong>Current Task</strong> — the emphasized row in the middle. Above it, every <strong>ancestor</strong> is stacked as a thin context bar (root at the top, down to the immediate parent); below it, the task's <strong>descendant subtree</strong> is drawn one band per level (<strong>Subtasks · L1</strong>, <strong>L2</strong>, …), loaded level by level.</p>
      <p>Each task occupies one row:</p>
      <ul className="space-y-1.5">
        <Term label="Label">the task's kind, drawn above its bar.</Term>
        <Term label="Bar">the task's span on the time axis (focused on the current task's window; ancestors that run far wider are clamped to the chart as context).</Term>
        <Term label="Wavy line below">the task's <strong>blocking intervals</strong>, colored by reason — drawn under any task that blocked (see <em>Blocking reasons</em>).</Term>
      </ul>
      <p>Interact with it directly:</p>
      <ul className="space-y-1.5">
        <Term label="Click a bar">select that task — its details fill the side panel and its kind is highlighted in the legend.</Term>
        <Term label="Double-click a bar">re-center the view on that task, making it the new Current Task (its own ancestors and subtasks reload around it).</Term>
        <Term label="Click a wavy line">inspect that blocking interval — its reason and how long the task waited.</Term>
        <Term label="Drag / scroll">pan the time axis; <strong>⌘/Ctrl+scroll</strong> zooms it. <strong>Expand next level</strong> loads another layer of subtasks.</Term>
      </ul>
      <p>The side panel shows the selected task (or blocking reason), and the legend lists the task kinds (see <em>Task types</em>) and blocking reasons present, with the <strong>Kind / Kind-What</strong> coloring toggle.</p>
    </InfoButton>
  );
}

// TaskHierarchyHelp explains the selected-task panel: a task shown together with its
// parent task and sub-tasks on one time axis (the task tree).
export function TaskHierarchyHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Task hierarchy" className={className}>
      <Figure src={taskTreeImg} alt="A selected task shown with its parent task and sub-tasks on one time axis" />
      <p>Every task is part of a tree. A component serves a request (a task) by issuing its own downstream requests (sub-tasks), which other components serve in turn — so the work fans out and comes back.</p>
      <p>When you <strong>select a task</strong> — click it in the timeline, or open a task link — this panel shows that task in the middle (<strong>Current task</strong>) together with:</p>
      <ul className="space-y-1.5">
        <Term label="Parent task">the upstream request that caused it — the work that was waiting on this task to finish.</Term>
        <Term label="Sub-tasks">the downstream requests this task issued while serving its own — what it, in turn, waited on.</Term>
      </ul>
      <p>All three rows share <strong>one time axis</strong>, so you can see when the task started relative to its parent, and which sub-task it was waiting on at any moment. Click the parent or a sub-task to walk up or down the tree.</p>
      <p>The wavy line under the current task marks its <strong>blocking intervals</strong> — see Blocking reasons.</p>
    </InfoButton>
  );
}

// BlockingReasonsHelp explains the component view's blocking-reason chart + milestones.
export function BlockingReasonsHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Blocking reasons" className={className}>
      <Figure src={blockingImg} alt="The blocking-reasons chart: in-flight blocked tasks stacked and colored by reason over time" />
      <p>A task is <strong>blocked</strong> whenever it is waiting on something — a buffer slot, a data response, an address translation, an ordering dependency, and so on. Each wait is recorded as a <strong>milestone</strong> that marks the moment the wait ended; the span leading up to it is time spent blocked on that reason.</p>
      <p>The <strong>blocking-reasons chart</strong> (bottom) shows, at each sampled time, how many in-flight tasks are blocked by each reason — stacked and colored by reason. A tall single-color band means many tasks stalled on the same thing at that moment.</p>
      <p>The colors match the wavy lines drawn under a selected task's bar (each wave is one blocking interval, ending at its milestone). Hover a band to highlight, in the timeline above, the tasks blocked by that reason at that time.</p>
      <p className="font-medium text-foreground">Common reasons</p>
      <ul className="space-y-1.5">
        <Term label="queue">waiting behind other messages in a queue or buffer — the message is in line, not yet at the head.</Term>
        <Term label="hardware_resource">waiting for a hardware resource to free up — a buffer slot, a cache bank, an MSHR, or a pipeline stage.</Term>
        <Term label="network_busy">waiting to send on a busy port — the message is ready but the outgoing link/connection is occupied.</Term>
        <Term label="network_transfer">the message is in transit across the network between components.</Term>
        <Term label="data">waiting for a data response to return from a downstream component (e.g. a cache or memory read).</Term>
        <Term label="translation">waiting for an address translation (a TLB lookup or page-table walk) to resolve.</Term>
        <Term label="dependency">waiting on an ordering or data dependency — e.g. an in-order commit that cannot proceed until an earlier request does.</Term>
        <Term label="subtask">waiting on a child sub-task to finish — work this task handed off and now depends on.</Term>
        <Term label="work"><em>not</em> a wait: the interval was spent doing productive internal work (backed by a sub-task showing what), so it is time working rather than blocked.</Term>
      </ul>
    </InfoButton>
  );
}
