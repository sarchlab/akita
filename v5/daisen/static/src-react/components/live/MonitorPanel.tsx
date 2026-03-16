import { useCallback, useState } from "react";
import { useMode } from "../../hooks/useMode";
import MonitorWidget from "./MonitorWidget";

/** Identifies one monitored field. */
interface WidgetDef {
  component: string;
  field: string;
}

/**
 * MonitorPanel — container for MonitorWidget instances.
 * Widgets are laid out horizontally and auto-sized to fill width.
 * Only visible in live mode when at least one widget exists.
 */
export default function MonitorPanel() {
  const { mode } = useMode();
  const [widgets, setWidgets] = useState<WidgetDef[]>([]);

  const addWidget = useCallback((component: string, field: string) => {
    setWidgets((prev) => {
      const exists = prev.some(
        (w) => w.component === component && w.field === field,
      );
      if (exists) return prev;
      return [...prev, { component, field }];
    });
  }, []);

  const removeWidget = useCallback((component: string, field: string) => {
    setWidgets((prev) =>
      prev.filter((w) => !(w.component === component && w.field === field)),
    );
  }, []);

  if (mode !== "live") return null;

  // Expose addWidget on the window for external callers
  // (e.g. ComponentDetailView may call window.__addMonitorWidget)
  if (typeof window !== "undefined") {
    (window as unknown as Record<string, unknown>).__addMonitorWidget =
      addWidget;
  }

  if (widgets.length === 0) return null;

  return (
    <div className="border-top mt-3 pt-2">
      <h6 className="mb-2">Field Monitors</h6>
      <div className="d-flex gap-2 flex-wrap">
        {widgets.map((w) => (
          <MonitorWidget
            key={`${w.component}:${w.field}`}
            component={w.component}
            field={w.field}
            onClose={() => removeWidget(w.component, w.field)}
          />
        ))}
      </div>
    </div>
  );
}
