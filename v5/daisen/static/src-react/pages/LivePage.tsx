import { useCallback, useState } from "react";
import { useMode } from "../hooks/useMode";
import PanelLayout from "../components/layout/PanelLayout";
import ComponentTree from "../components/live/ComponentTree";
import ComponentDetailView from "../components/live/ComponentDetailView";
import HangAnalyzer from "../components/live/HangAnalyzer";
import ResourceMonitor from "../components/live/ResourceMonitor";
import MonitorPanel from "../components/live/MonitorPanel";
import ProgressBars from "../components/live/ProgressBars";

/**
 * LivePage — three-pane resizable layout for live mode.
 *
 *  Left:   Component tree
 *  Center: Component detail view
 *  Right:  Tools (Hang analyzer, resource monitor)
 *  Bottom: Monitor widgets & progress bars
 *
 * Uses PanelLayout with draggable dividers ported from ui_manager.ts.
 */
export default function LivePage() {
  const { mode, loading } = useMode();
  const [selected, setSelected] = useState<string | null>(null);

  const handleSelect = useCallback((fullName: string) => {
    setSelected(fullName);
  }, []);

  const handleMonitor = useCallback(
    (componentName: string, keyChain: string) => {
      console.log("Monitor toggle:", componentName, keyChain);
      // Integrate with MonitorPanel via window.__addMonitorWidget
      const addFn = (
        window as unknown as Record<string, unknown>
      ).__addMonitorWidget;
      if (typeof addFn === "function") {
        (addFn as (c: string, f: string) => void)(componentName, keyChain);
      }
    },
    [],
  );

  if (loading) {
    return (
      <div className="p-3 text-muted">
        <div className="spinner-border spinner-border-sm me-2" />
        Checking mode…
      </div>
    );
  }

  if (mode !== "live") {
    return (
      <div className="container py-4">
        <h4>Live Inspector</h4>
        <p className="text-muted">
          This page is only available in <strong>live</strong> mode. Current
          mode: <code>{mode ?? "unknown"}</code>.
        </p>
      </div>
    );
  }

  /* ── Panel contents ─────────────────────────────────────── */

  const leftPanel = (
    <div className="p-2">
      <h6 className="text-muted mb-2">Components</h6>
      <ComponentTree onSelectComponent={handleSelect} />
    </div>
  );

  const centerPanel = selected ? (
    <div className="p-3">
      <ComponentDetailView
        componentName={selected}
        onMonitor={handleMonitor}
      />
    </div>
  ) : (
    <div className="text-muted mt-4 text-center">
      <i className="fas fa-arrow-left me-2" />
      Select a component from the tree to inspect it.
    </div>
  );

  const rightPanel = (
    <div className="p-2">
      <div className="mb-3">
        <HangAnalyzer />
      </div>
      <div>
        <ResourceMonitor />
      </div>
    </div>
  );

  const bottomPanel = (
    <div className="p-2">
      <ProgressBars />
      <MonitorPanel />
    </div>
  );

  return (
    <PanelLayout
      left={leftPanel}
      center={centerPanel}
      right={rightPanel}
      bottom={bottomPanel}
      showBottom={true}
    />
  );
}
