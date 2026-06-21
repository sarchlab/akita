import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import BlockedComponentsWidget from "../components/BlockedComponentsWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";

// MainPage is Daisen's landing page (route "/"): an at-a-glance overview of the
// loaded simulation in a 2x2 grid of equally sized widgets — run info, the
// component topology, the most blocked components, and the recorded source —
// each enlargeable to its own page.
export default function MainPage() {
  return (
    <div className="grid h-full grid-cols-2 grid-rows-2 gap-3 overflow-hidden bg-white p-3">
      <SimulationInfoWidget expandHref="/view/siminfo" />
      <TopologyWidget expandHref="/view/topology" />
      <BlockedComponentsWidget expandHref="/view/blocked" />
      <CodeBrowserWidget expandHref="/view/code" />
    </div>
  );
}
