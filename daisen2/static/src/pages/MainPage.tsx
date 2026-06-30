import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import ComponentsWidget from "../components/ComponentsWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";
import DatabaseWidget from "../components/DatabaseWidget";
import BlockingResourcesWidget from "../components/BlockingResourcesWidget";

// MainPage is Daisen's landing page (route "/"): a fixed, single-screen overview
// of the loaded simulation — the page itself never scrolls (each widget scrolls
// its own content). A 6-column, 2-row grid puts six widgets at equal thirds: the
// top row carries the three compact panels (simulation, database, source) and the
// bottom row the three trace views (components, topology, top blocking resources).
// Each widget is enlargeable to its own page.
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
      <div className="col-span-2 flex min-h-0 min-w-0">
        <ComponentsWidget expandHref="/dashboard" />
      </div>
      <div className="col-span-2 flex min-h-0 min-w-0">
        <TopologyWidget expandHref="/view/topology" />
      </div>
      <div className="col-span-2 flex min-h-0 min-w-0">
        <BlockingResourcesWidget expandHref="/view/blocking" />
      </div>
    </div>
  );
}
