import InfoButton from "./InfoButton";
import metricsImg from "../assets/help/metrics.png";
import componentsImg from "../assets/help/components.png";
import tasksImg from "../assets/help/tasks.png";
import blockingImg from "../assets/help/blocking.png";
import taskTreeImg from "../assets/help/task-tree.png";
import componentTasksImg from "../assets/help/component-tasks.png";

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

// ComponentsOverviewHelp explains the index page's Components widget: residency
// ranking, auto-selected metrics, and the aggregate facet badge.
export function ComponentsOverviewHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Components overview" className={className}>
      <Figure src={componentsImg} alt="The Components overview: four component charts with facet badges and per-chart legends" />
      <p>The components with the highest <strong>residency</strong> — the total time their tasks spend in flight, summed across the run. It is a cheap proxy for where the simulation spends time, so the busiest / most-contended components rank first.</p>
      <p>Each chart's two metrics are <strong>auto-selected</strong> for that component: components that serve requests show <strong>Incoming Request Rate</strong>; pure clients (which only issue requests) show <strong>Response Buffer Pressure</strong>. The other line is always <strong>Average Request Latency</strong>. The colored dots below each chart name them.</p>
      <p>A <strong>Σ N facets</strong> badge means the chart sums the component's whole subtree — its N <em>one-location-one-kind</em> facets (e.g. <code>AT.req_in</code>, <code>AT.Top.incoming</code>, …). Open a chart to drill into the component.</p>
    </InfoButton>
  );
}

// ComponentTasksHelp explains the per-task view itself: all of a component's tasks,
// the task-count summary when zoomed out, and the individual task bars when zoomed in.
export function ComponentTasksHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Component tasks" className={className}>
      <p>This view shows <strong>all of the component's tasks</strong> over the visible time range. A task is one unit of work the component did — serving an incoming request (<code>req_in</code>), a request it sent out (<code>req_out</code>), the time a message spent in a buffer (<code>incoming_buffer</code> / <code>outgoing_buffer</code>), a pipeline stage, and so on.</p>
      <Figure
        src={tasksImg}
        alt="The task-count chart: in-flight tasks stacked and colored over time"
        caption="Zoomed out — the task-count chart: how many tasks are in flight at each moment."
      />
      <p>When many tasks fall in the range they are summarized as a <strong>task-count chart</strong>: at each time it stacks how many tasks are in flight, so the stack height is the component's <strong>concurrency</strong>. A stack that stays high with no dips means the component is working at full capacity.</p>
      <Figure
        src={componentTasksImg}
        alt="The per-task gantt: each task drawn as its own bar across time"
        caption="Zoomed in — each task becomes its own bar. Hover for details; click to select."
      />
      <p><strong>Zoom in</strong> (⌘/Ctrl + scroll, pinch, or drag-select a range) until few enough tasks remain that each is drawn as its own <strong>bar</strong>. Hover a bar for its details, and <strong>click</strong> one to select it — that opens its <em>task hierarchy</em> above and its full details in the side panel. Scroll or drag to pan.</p>
      <p>The legend's <strong>Kind / Kind-What</strong> toggle controls the coloring — by task kind alone, or split further by the message type (e.g. <code>incoming_buffer · ReadReq</code> vs <code>· WriteReq</code>). It recolors both these bars and the task-count bands.</p>
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
    </InfoButton>
  );
}
