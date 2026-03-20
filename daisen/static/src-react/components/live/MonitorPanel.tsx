import { useCallback, useEffect, useState } from "react";
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
export default function MonitorPanel({
  onCountChange,
}: {
  onCountChange?: (count: number) => void;
}) {
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

  useEffect(() => {
    if (typeof window === "undefined") return;

    const monitorWindow = window as unknown as Record<string, unknown>;

    if (mode !== "live") {
      monitorWindow.__addMonitorWidget = undefined;
      monitorWindow.__removeMonitorWidget = undefined;
      return;
    }

    monitorWindow.__addMonitorWidget = addWidget;
    monitorWindow.__removeMonitorWidget = removeWidget;

    return () => {
      if (monitorWindow.__addMonitorWidget === addWidget) {
        monitorWindow.__addMonitorWidget = undefined;
      }
      if (monitorWindow.__removeMonitorWidget === removeWidget) {
        monitorWindow.__removeMonitorWidget = undefined;
      }
    };
  }, [mode, addWidget, removeWidget]);

  useEffect(() => {
    onCountChange?.(widgets.length);
  }, [widgets.length, onCountChange]);

  if (mode !== "live") return null;
  if (widgets.length === 0) return null;

  return (
    <div className="flex-grow-1 d-flex flex-column" style={{ minHeight: 0 }}>
      <div className="d-flex gap-2 flex-grow-1" style={{ minHeight: 0 }}>
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
