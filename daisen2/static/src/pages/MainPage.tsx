import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import ComponentsWidget from "../components/ComponentsWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";
import DatabaseWidget from "../components/DatabaseWidget";

// MainPage is Daisen's landing page (route "/"): a fixed, single-screen overview
// of the loaded simulation — the page itself never scrolls (each widget scrolls
// its own content). A 6-column grid lets the two rows use different widget
// counts at equal widths: the top row carries the three compact panels
// (simulation, database, source) at thirds, the bottom row gives the two
// time-series views (components, topology) the full half-width each. Each widget
// is enlargeable to its own page.
export default function MainPage() {
  return (
    <div className="grid h-full grid-cols-6 grid-rows-2 gap-3 overflow-hidden bg-white p-3">
      <div className="col-span-2 flex min-h-0 min-w-0">
        <SimulationInfoWidget expandHref="/view/siminfo" />
      </div>
      <div className="col-span-2 flex min-h-0 min-w-0">
        <DatabaseWidget expandHref="/view/database" />
      </div>
      <div className="col-span-2 flex min-h-0 min-w-0">
        <CodeBrowserWidget expandHref="/view/code" />
      </div>
      <div className="col-span-3 flex min-h-0 min-w-0">
        <ComponentsWidget expandHref="/dashboard" />
      </div>
      <div className="col-span-3 flex min-h-0 min-w-0">
        <TopologyWidget expandHref="/view/topology" />
      </div>
    </div>
  );
}
