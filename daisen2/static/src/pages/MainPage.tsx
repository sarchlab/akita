import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import BlockedComponentsWidget from "../components/BlockedComponentsWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";

// MainPage is Daisen's landing page (route "/"): a weighted 2x2 overview of the
// loaded simulation. The top row pairs the compact run info with a wide
// topology; the bottom row gives the blocked-component charts the width and the
// source browser a column. Each widget is enlargeable to its own page.
export default function MainPage() {
  return (
    <div className="grid h-full grid-cols-3 grid-rows-2 gap-3 overflow-hidden bg-white p-3">
      <div className="flex min-h-0 min-w-0">
        <SimulationInfoWidget expandHref="/view/siminfo" />
      </div>
      <div className="col-span-2 flex min-h-0 min-w-0">
        <TopologyWidget expandHref="/view/topology" />
      </div>
      <div className="col-span-2 flex min-h-0 min-w-0">
        <BlockedComponentsWidget expandHref="/dashboard" />
      </div>
      <div className="flex min-h-0 min-w-0">
        <CodeBrowserWidget expandHref="/view/code" />
      </div>
    </div>
  );
}
