import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import ComponentsWidget from "../components/ComponentsWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";
import DatabaseWidget from "../components/DatabaseWidget";
import BlockingResourcesWidget from "../components/BlockingResourcesWidget";

// MainPage is Daisen's landing page (route "/"): an overview of the loaded
// simulation. Widgets keep a usable minimum width and the page scrolls vertically
// when the viewport cannot fit the full two-row overview.
export default function MainPage() {
  return (
    <div className="daisen-overview-grid">
      <div className="flex min-h-0 min-w-0">
        <SimulationInfoWidget expandHref="/view/siminfo" />
      </div>
      <div className="flex min-h-0 min-w-0">
        <DatabaseWidget expandHref="/view/database" />
      </div>
      <div className="flex min-h-0 min-w-0">
        <CodeBrowserWidget expandHref="/view/code" />
      </div>
      <div className="flex min-h-0 min-w-0">
        <ComponentsWidget expandHref="/dashboard" />
      </div>
      <div className="flex min-h-0 min-w-0">
        <TopologyWidget expandHref="/view/topology" />
      </div>
      <div className="flex min-h-0 min-w-0">
        <BlockingResourcesWidget expandHref="/view/blocking" />
      </div>
    </div>
  );
}
