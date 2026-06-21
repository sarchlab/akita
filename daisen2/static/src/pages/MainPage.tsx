import { Link } from "react-router-dom";
import { LayoutDashboard } from "lucide-react";
import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import BlockedComponentsWidget from "../components/BlockedComponentsWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";
import { Button } from "../components/ui/button";

// MainPage is Daisen's landing page (route "/"): an at-a-glance overview of the
// loaded simulation — run info, component/connection topology, the most blocked
// components, and the recorded source — each widget enlargeable to its own page.
export default function MainPage() {
  return (
    <div className="flex h-full flex-col gap-3 overflow-hidden bg-white p-3">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Overview</h1>
        <Button asChild variant="outline" size="sm">
          <Link to="/dashboard">
            <LayoutDashboard />
            Open dashboard
          </Link>
        </Button>
      </div>

      <div className="shrink-0">
        <SimulationInfoWidget expandHref="/view/siminfo" />
      </div>

      <div className="flex min-h-0 flex-1 gap-3">
        <TopologyWidget expandHref="/view/topology" />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
          <BlockedComponentsWidget expandHref="/view/blocked" />
          <CodeBrowserWidget expandHref="/view/code" />
        </div>
      </div>
    </div>
  );
}
