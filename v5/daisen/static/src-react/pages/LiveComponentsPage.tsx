import { useCallback, useState } from "react";
import { useMode } from "../hooks/useMode";
import ComponentTree from "../components/live/ComponentTree";
import ComponentDetailView from "../components/live/ComponentDetailView";

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
    (componentName: string, keyChain: string) => {
      console.log("Monitor toggle:", componentName, keyChain);
      // Future: integrate with a monitor widget system
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
    <div className="d-flex" style={{ height: "calc(100vh - 70px)" }}>
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
  );
}
