import InfoButton from "./InfoButton";
import metricsImg from "../assets/help/metrics.png";
import componentsImg from "../assets/help/components.png";
import tasksImg from "../assets/help/tasks.png";
import blockingImg from "../assets/help/blocking.png";

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

// Figure shows an illustrating screenshot at the top of a help modal.
const Figure = ({ src, alt }: { src: string; alt: string }) => (
  <img src={src} alt={alt} className="mb-2 w-full rounded border border-slate-200" loading="lazy" />
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

// TaskColoringHelp explains the task-count chart and the kind / kind-what coloring.
export function TaskColoringHelp({ className }: { className?: string }) {
  return (
    <InfoButton title="Tasks &amp; coloring" className={className}>
      <Figure src={tasksImg} alt="The task-count chart: in-flight tasks stacked and colored over time" />
      <p>The <strong>task-count chart</strong> shows, at each time, how many of the component's tasks are in flight, stacked and colored. The stack height is the component's concurrency; a stack that stays high with no dips suggests the component is working at full capacity.</p>
      <p>Choose how the tasks (and these bands) are colored:</p>
      <ul className="space-y-1.5">
        <Term label="Kind">color by task kind alone — <code>req_in</code>, <code>req_out</code>, <code>incoming_buffer</code>, …</Term>
        <Term label="Kind-What">also split each kind by the message type, e.g. <code>incoming_buffer · ReadReq</code> vs <code>· WriteReq</code>.</Term>
      </ul>
    </InfoButton>
  );
}
