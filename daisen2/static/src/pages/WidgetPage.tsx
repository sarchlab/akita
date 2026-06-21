import { type ComponentType } from "react";
import { useParams } from "react-router-dom";
import SimulationInfoWidget from "../components/SimulationInfoWidget";
import TopologyWidget from "../components/TopologyWidget";
import CodeBrowserWidget from "../components/CodeBrowserWidget";

// The overview widgets that have their own enlarged page, keyed by the id used
// in /view/<id>. (The blocked-components widget expands to /dashboard instead.)
// Each is self-contained, so it renders the same enlarged or in a card; the
// enlarged form is rendered "bare" — full-bleed, with no card frame.
const WIDGETS: Record<string, ComponentType<{ bare?: boolean }>> = {
  siminfo: SimulationInfoWidget,
  topology: TopologyWidget,
  code: CodeBrowserWidget,
};

// WidgetPage renders a single overview widget as a full page (route
// /view/:widget): just the widget, no surrounding chrome.
export default function WidgetPage() {
  const { widget } = useParams();
  const Widget = widget ? WIDGETS[widget] : undefined;

  if (!Widget) {
    return (
      <div className="flex h-full items-center justify-center bg-white text-sm text-muted-foreground">
        Unknown widget “{widget}”.
      </div>
    );
  }

  return (
    <div className="h-full min-h-0 bg-white">
      <Widget bare />
    </div>
  );
}
