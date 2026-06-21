import { Link } from "react-router-dom";
import { LayoutDashboard } from "lucide-react";
import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import { Button } from "../components/ui/button";
import { useTopology } from "../hooks/useTopology";
import type { Topology } from "../types/overview";

const EMPTY_TOPOLOGY: Topology = { components: [], ports: [] };

// MainPage is Daisen's landing page (route "/"): an at-a-glance overview of the
// loaded simulation — basic run info plus its component/connection topology —
// from which the user can drill into the dashboard.
export default function MainPage() {
  const { data, loading, error } = useTopology();
  const topology = data ?? EMPTY_TOPOLOGY;

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

      <SimulationInfoWidget componentCount={data ? topology.components.length : undefined} />

      <TopologyWidget topology={topology} loading={loading} error={error} />
    </div>
  );
}
