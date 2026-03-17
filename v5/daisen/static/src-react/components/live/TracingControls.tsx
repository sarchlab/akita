import { useCallback, useEffect, useState } from "react";
import { useMode } from "../../hooks/useMode";

/**
 * Start / Stop tracing toggle.  Checks the initial state via
 * GET /api/trace/is_tracing and toggles via POST /api/trace/start
 * and POST /api/trace/end.  Only renders when mode === "live".
 */
export default function TracingControls() {
  const { mode } = useMode();
  const [tracing, setTracing] = useState(false);
  const [busy, setBusy] = useState(false);

  /* Fetch initial tracing state */
  useEffect(() => {
    const controller = new AbortController();

    fetch("/api/trace/is_tracing", { signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((data: { isTracing: boolean }) => {
        setTracing(Boolean(data.isTracing));
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        console.error("Error fetching trace status:", err);
      });

    return () => controller.abort();
  }, []);

  const toggle = useCallback(async () => {
    if (busy) return;
    setBusy(true);
    try {
      if (tracing) {
        await fetch("/api/trace/end", { method: "POST" });
        setTracing(false);
      } else {
        await fetch("/api/trace/start", { method: "POST" });
        setTracing(true);
      }
    } catch (err) {
      console.error("Error toggling trace:", err);
    } finally {
      setBusy(false);
    }
  }, [tracing, busy]);

  if (mode !== "live") return null;

  return (
    <div className="btn-group btn-group-sm" role="group">
      {/* Icon indicator */}
      <button
        type="button"
        className={`btn ${tracing ? "btn-success" : "btn-danger"}`}
        disabled
        aria-label="Tracing indicator"
      >
        <i
          className={`fas fa-circle-notch${tracing ? " fa-spin" : ""}`}
        />
      </button>

      {/* Toggle button */}
      <button
        type="button"
        className={`btn ${tracing ? "btn-danger" : "btn-success"}`}
        onClick={toggle}
        disabled={busy}
      >
        {tracing ? "Stop Tracing" : "Start Tracing"}
      </button>
    </div>
  );
}
