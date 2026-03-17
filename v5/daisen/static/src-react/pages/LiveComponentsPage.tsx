import { useCallback, useState } from "react";
import { useMode } from "../hooks/useMode";
import ComponentTree from "../components/live/ComponentTree";
import ComponentDetailView from "../components/live/ComponentDetailView";
import MonitorPanel from "../components/live/MonitorPanel";

/**
 * Live Components page — two-pane layout:
 *  - Left: component tree (fetched from /api/list_components)
 *  - Right: component detail view (fetched when a leaf is selected)
 *
 * Only renders content when mode === "live".
 */
export default function LiveComponentsPage() {
  const { mode, loading } = useMode();
  const [selected, setSelected] = useState<string | null>(null);

  const handleSelect = useCallback((fullName: string) => {
    setSelected(fullName);
  }, []);

  const handleMonitor = useCallback(
    (componentName: string, keyChain: string, selected: boolean) => {
      const monitorWindow = window as unknown as Record<string, unknown>;
      const action = selected
        ? monitorWindow.__addMonitorWidget
        : monitorWindow.__removeMonitorWidget;

      if (typeof action === "function") {
        (action as (c: string, f: string) => void)(componentName, keyChain);
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
        <h4>Live Component Inspector</h4>
        <p className="text-muted">
          This page is only available in <strong>live</strong> mode. Current
          mode: <code>{mode ?? "unknown"}</code>.
        </p>
      </div>
    );
  }

  return (
    <div className="d-flex flex-column" style={{ height: "calc(100vh - 70px)" }}>
      <div className="d-flex flex-grow-1 overflow-hidden">
        {/* Left pane — tree */}
        <div
          className="border-end overflow-auto p-2"
          style={{ width: 320, minWidth: 220, flexShrink: 0 }}
        >
          <h6 className="text-muted mb-2">Components</h6>
          <ComponentTree onSelectComponent={handleSelect} />
        </div>

        {/* Right pane — detail */}
        <div className="flex-grow-1 overflow-auto p-3">
          {selected ? (
            <ComponentDetailView
              componentName={selected}
              onMonitor={handleMonitor}
            />
          ) : (
            <div className="text-muted mt-4 text-center">
              <i className="fas fa-arrow-left me-2" />
              Select a component from the tree to inspect it.
            </div>
          )}
        </div>
      </div>

      <div className="px-3">
        <MonitorPanel />
      </div>
    </div>
  );
}
