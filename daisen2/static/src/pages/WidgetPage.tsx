import { type ComponentType } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import BlockedComponentsWidget from "../components/BlockedComponentsWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";

// The overview widgets, keyed by the id used in /view/<id>. Each is self-
// contained (fetches its own data), so it renders the same enlarged or in a
// card; the enlarged form just omits the expand button.
const WIDGETS: Record<string, ComponentType> = {
  siminfo: SimulationInfoWidget,
  topology: TopologyWidget,
  blocked: BlockedComponentsWidget,
  code: CodeBrowserWidget,
};

// WidgetPage renders a single overview widget full-screen (route /view/:widget),
// the enlarged form reached from each card's expand button.
export default function WidgetPage() {
  const { widget } = useParams();
  const Widget = widget ? WIDGETS[widget] : undefined;

  return (
    <div className="flex h-full flex-col gap-3 overflow-hidden bg-white p-3">
      <Link
        to="/"
        className="flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Overview
      </Link>

      {Widget ? (
        <div className="flex min-h-0 flex-1">
          <Widget />
        </div>
      ) : (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          Unknown widget “{widget}”.
        </div>
      )}
    </div>
  );
}
